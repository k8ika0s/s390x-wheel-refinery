from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from .history import BuildHistory
from .web import _auth_guard, _run_worker_once, _worker_settings_from_env


def _worker_token() -> Optional[str]:
    return os.environ.get("WORKER_TOKEN")


def _build_history() -> BuildHistory:
    env_path = os.environ.get("WORKER_HISTORY_DB") or os.environ.get("HISTORY_DB")
    path = Path(env_path) if env_path else Path("/cache/history.db")
    return BuildHistory(path)


app = FastAPI(title="s390x Wheel Refinery Worker")
settings = _worker_settings_from_env(_build_history().path.parent)


@app.get("/healthz")
def health():
    return {"status": "ok", "worker_configured": settings is not None}


@app.post("/trigger")
async def trigger(request: Request):
    token = _worker_token()
    auth_error = _auth_guard(token, request)
    if auth_error:
        return auth_error
    if not settings:
        return JSONResponse(status_code=503, content={"detail": "Worker paths not configured"})
    ok, detail = await _run_worker_once(settings)
    status = 200 if ok else 500
    return JSONResponse(status_code=status, content={"detail": detail})
