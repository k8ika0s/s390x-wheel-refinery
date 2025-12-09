from __future__ import annotations

import asyncio
import os
import contextlib
import json as jsonlib
import urllib.request
from pathlib import Path
from typing import List, Optional
from dataclasses import dataclass
from contextlib import asynccontextmanager

from fastapi import FastAPI, Query, Request, Response
from pydantic import BaseModel
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, StreamingResponse

from .history import BuildHistory, BuildEvent, FailureStat, PackageSummary
from .hints import HintCatalog, Hint
from .queue import RetryQueue, RetryRequest
from .worker import process_queue


@dataclass
class WorkerSettings:
    input_dir: Path
    output_dir: Path
    cache_dir: Path
    history_path: Path
    python_version: str
    platform_tag: str
    container_image: Optional[str]
    container_preset: Optional[str]


def _worker_settings_from_env(history_dir: Path) -> Optional[WorkerSettings]:
    input_dir = Path(os.environ.get("WORKER_INPUT_DIR", "/input"))
    output_dir = Path(os.environ.get("WORKER_OUTPUT_DIR", "/output"))
    cache_dir = Path(os.environ.get("WORKER_CACHE_DIR", "/cache"))
    python_version = os.environ.get("WORKER_PYTHON", "3.11")
    platform_tag = os.environ.get("WORKER_PLATFORM_TAG", "manylinux2014_s390x")
    container_image = os.environ.get("WORKER_CONTAINER_IMAGE")
    container_preset = os.environ.get("WORKER_CONTAINER_PRESET")
    history_path = Path(os.environ.get("WORKER_HISTORY_DB", history_dir / "history.db"))

    for path in (input_dir, output_dir, cache_dir):
        if not path.exists():
            return None
    return WorkerSettings(
        input_dir=input_dir,
        output_dir=output_dir,
        cache_dir=cache_dir,
        history_path=history_path,
        python_version=python_version,
        platform_tag=platform_tag,
        container_image=container_image,
        container_preset=container_preset,
    )


def _auth_guard(token: Optional[str], request: Request):
    if not token:
        return None
    header = request.headers.get("x-worker-token")
    query = request.query_params.get("token")
    cookie = request.cookies.get("worker_token")
    if header == token or query == token or cookie == token:
        return None
    return JSONResponse(status_code=403, content={"detail": "forbidden"})


async def _run_worker_once(settings: WorkerSettings) -> tuple[bool, str]:
    try:
        await asyncio.to_thread(
            process_queue,
            input_dir=settings.input_dir,
            output_dir=settings.output_dir,
            cache_dir=settings.cache_dir,
            history_path=settings.history_path,
            python_version=settings.python_version,
            platform_tag=settings.platform_tag,
            container_image=settings.container_image,
            container_preset=settings.container_preset,
        )
        return True, "worker ran"
    except Exception as exc:  # noqa: BLE001
        return False, str(exc)


async def _trigger_worker_webhook(url: str, token: Optional[str]) -> tuple[bool, str]:
    def _call():
        req = urllib.request.Request(url, method="POST")
        if token:
            req.add_header("X-Worker-Token", token)
        req.add_header("Content-Type", "application/json")
        data = jsonlib.dumps({"action": "drain"}).encode()
        with urllib.request.urlopen(req, data=data, timeout=30) as resp:  # noqa: S310
            if resp.status >= 400:
                raise RuntimeError(f"Webhook responded {resp.status}")
            return resp.read().decode()

    last_exc: Exception | None = None
    for _ in range(2):  # simple retry
        try:
            body = await asyncio.to_thread(_call)
            return True, body or "worker triggered via webhook"
        except Exception as exc:  # noqa: BLE001
            last_exc = exc
            await asyncio.sleep(1)
    return False, str(last_exc) if last_exc else "webhook failed"


def create_app(history: BuildHistory) -> FastAPI:
    hint_catalog = HintCatalog(Path(__file__).parent.parent / "data" / "hints.yaml")
    retry_queue = RetryQueue(history.path.parent / "retry_queue.json")
    worker_settings = _worker_settings_from_env(history.path.parent)
    worker_webhook = os.environ.get("WORKER_WEBHOOK_URL")
    worker_token = os.environ.get("WORKER_TOKEN")
    worker_mode = "webhook" if worker_webhook else ("local" if worker_settings else None)

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        task = None
        interval = os.environ.get("WORKER_AUTORUN_INTERVAL")
        if interval and worker_settings and not worker_webhook:
            try:
                seconds = int(interval)
            except ValueError:
                seconds = None
            if seconds:
                async def _loop():
                    while True:
                        try:
                            await _run_worker_once(worker_settings)
                        except Exception:
                            print("worker autorun failed", flush=True)
                        await asyncio.sleep(seconds)

                task = asyncio.create_task(_loop())
                app.state.worker_task = task
        yield
        if task:
            task.cancel()
            with contextlib.suppress(Exception):
                await task

    app = FastAPI(title="s390x Wheel Refinery History", lifespan=lifespan)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    class RetryPayload(BaseModel):
        version: str | None = "latest"
        python_tag: str | None = None
        platform_tag: str | None = None
        recipes: List[str] | None = None

    @app.get("/")
    def health():
        return {"status": "ok"}

    @app.get("/api/recent")
    def api_recent(
        limit: int = Query(20, le=200),
        status: Optional[str] = None,
        package: Optional[str] = None,
    ) -> List[BuildEvent]:
        events = history.recent(limit=limit, status=status)
        if package:
            events = [event for event in events if event.name.lower() == package.lower()]
        return events

    @app.get("/api/top-failures")
    def api_top_failures(limit: int = Query(20, le=200)) -> List[FailureStat]:
        return history.top_failures(limit=limit)

    @app.get("/api/top-slowest")
    def api_top_slowest(limit: int = Query(10, le=200)):
        return history.top_slowest(limit=limit)

    @app.get("/api/hints")
    def api_hints() -> List[Hint]:
        return hint_catalog.hints

    @app.get("/api/failures")
    def api_failures(limit: int = Query(50, le=500), name: Optional[str] = None):
        return history.failures_over_time(name=name, limit=limit)

    @app.get("/api/variants/{name}")
    def api_variants(name: str, limit: int = Query(100, le=500)):
        return history.variant_history(name, limit=limit)

    @app.get("/api/package/{name}")
    def api_package(name: str) -> PackageSummary:
        return history.package_summary(name)

    @app.get("/api/event/{name}/{version}")
    def api_event(name: str, version: str):
        event = history.last_event(name, version)
        if not event:
            return JSONResponse(status_code=404, content={"detail": "not found"})
        return event

    @app.get("/api/summary")
    def api_summary():
        return {"status_counts": history.status_counts_recent(limit=50), "failures": history.recent_failures(limit=20)}

    @app.get("/api/metrics")
    def api_metrics(request: Request):
        auth_error = _auth_guard(worker_token, request)
        if auth_error:
            return auth_error
        return {
            "status_counts": history.status_counts_recent(limit=50),
            "failures": history.recent_failures(limit=20),
            "queue_length": len(retry_queue),
            "worker_mode": worker_mode,
        }

    @app.get("/api/queue")
    def api_queue(request: Request):
        auth_error = _auth_guard(worker_token, request)
        if auth_error:
            return auth_error
        return {"length": len(retry_queue), "worker_available": worker_mode is not None, "worker_mode": worker_mode}

    @app.post("/package/{name}/retry")
    async def retry_with_recipe(name: str, payload: RetryPayload, request: Request):
        auth_error = _auth_guard(worker_token, request)
        if auth_error:
            return auth_error
        recipe_steps = list(payload.recipes or [])
        if not recipe_steps:
            for hint in hint_catalog.hints:
                if hint.recipes:
                    recipe_steps.extend(hint.recipes.get("dnf", []))
                    recipe_steps.extend(hint.recipes.get("apt", []))
        req = RetryRequest(
            package=name,
            version=payload.version or "latest",
            python_tag=payload.python_tag or "cp311",
            platform_tag=payload.platform_tag or "manylinux2014_s390x",
            recipes=recipe_steps,
        )
        queue_length = retry_queue.add(req)
        return JSONResponse({"detail": f"Enqueued retry for {name}", "queue_length": queue_length})

    @app.post("/api/worker/trigger")
    async def trigger_worker(request: Request):
        auth_error = _auth_guard(worker_token, request)
        if auth_error:
            return auth_error
        if worker_webhook:
            ok, detail = await _trigger_worker_webhook(worker_webhook, worker_token)
            status = 200 if ok else 502
            return JSONResponse(status_code=status, content={"detail": detail})
        if not worker_settings:
            return JSONResponse(status_code=503, content={"detail": "Worker paths not configured"})
        ok, detail = await _run_worker_once(worker_settings)
        if not ok:
            return JSONResponse(status_code=500, content={"detail": detail})
        return {"detail": detail, "queue_cleared": True, "queue_length": len(retry_queue)}

    @app.post("/api/session/token")
    def set_token(token: str, response: Response):
        if not worker_token:
            return JSONResponse(status_code=404, content={"detail": "worker token not configured"})
        if token != worker_token:
            return JSONResponse(status_code=403, content={"detail": "forbidden"})
        # Set a cookie so UI calls can include the token without adding it to query params.
        response.set_cookie("worker_token", token, httponly=False, samesite="lax")
        return {"detail": "token set in cookie"}

    @app.get("/logs/{name}/{version}")
    def view_log(name: str, version: str):
        event = history.last_event(name, version)
        if not event or not event.metadata or not event.metadata.get("log_path"):
            return JSONResponse(status_code=404, content={"detail": "log not found"})
        log_path = Path(event.metadata["log_path"])
        if not log_path.exists():
            return JSONResponse(status_code=404, content={"detail": "log file missing"})
        content = log_path.read_text(errors="replace")
        return JSONResponse({"log_path": str(log_path), "content": content})

    @app.get("/logs/{name}/{version}/stream")
    async def stream_log(name: str, version: str):
        event = history.last_event(name, version)
        if not event or not event.metadata or not event.metadata.get("log_path"):
            return JSONResponse(status_code=404, content={"detail": "log not found"})
        log_path = Path(event.metadata["log_path"])
        if not log_path.exists():
            return JSONResponse(status_code=404, content={"detail": "log file missing"})

        def _generator():
            with log_path.open("r", encoding="utf-8", errors="replace") as fh:
                while True:
                    line = fh.readline()
                    if not line:
                        break
                    yield f"data: {line.rstrip()}\n\n"

        return StreamingResponse(_generator(), media_type="text/event-stream")

    @app.exception_handler(Exception)
    async def handle_exceptions(request: Request, exc: Exception):
        return JSONResponse(status_code=500, content={"detail": str(exc)})

    return app
