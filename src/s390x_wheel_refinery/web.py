from __future__ import annotations

from pathlib import Path
from typing import List, Optional

from fastapi import FastAPI, Query, Request
from fastapi.responses import JSONResponse, StreamingResponse
from fastapi.templating import Jinja2Templates

from .history import BuildHistory, BuildEvent, FailureStat, PackageSummary
from .hints import HintCatalog, Hint
from .resolver import build_plan
from .config import build_config
from .scanner import scan_wheels
from .manifest import write_manifest
from . import builder as builder_module

TEMPLATES = Jinja2Templates(directory=str(Path(__file__).parent / "templates"))


def create_app(history: BuildHistory) -> FastAPI:
    app = FastAPI(title="s390x Wheel Refinery History")
    hint_catalog = HintCatalog(Path(__file__).parent.parent / "data" / "hints.yaml")

    @app.get("/")
    def home(
        request: Request,
        recent: int = 20,
        top_failures: int = 10,
        status: Optional[str] = Query(None, description="Filter recent events by status"),
        package: Optional[str] = Query(None, description="Filter recent events by package name"),
    ):
        recent_events = history.recent(limit=recent, status=status)
        if package:
            recent_events = [event for event in recent_events if event.name.lower() == package.lower()]
        failures = history.top_failures(limit=top_failures)
        slowest = history.top_slowest(limit=10)
        status_counts = history.status_counts_recent(limit=50)
        recent_failures = history.recent_failures(limit=10)
        return TEMPLATES.TemplateResponse(
            "home.html",
            {
                "request": request,
                "recent": recent_events,
                "failures": failures,
                "slowest": slowest,
                "status_counts": status_counts,
                "recent_failures": recent_failures,
                "status_filter": status or "",
                "package_filter": package or "",
                "hint_catalog": hint_catalog.hints,
            },
        )

    @app.get("/package/{name}")
    def package_page(request: Request, name: str):
        summary = history.package_summary(name)
        events = history.recent(limit=200, status=None)
        package_events = [event for event in events if event.name.lower() == name.lower()]
        failures = history.failures_over_time(name=name, limit=50)
        variants = history.variant_history(name, limit=20)
        return TEMPLATES.TemplateResponse(
            "package.html",
            {
                "request": request,
                "summary": summary,
                "events": package_events,
                "failures": failures,
                "variants": variants,
            },
        )

    @app.get("/event/{name}/{version}")
    def event_detail(request: Request, name: str, version: str):
        event = history.last_event(name, version)
        if not event:
            return JSONResponse(status_code=404, content={"detail": "not found"})
        return TEMPLATES.TemplateResponse(
            "event.html",
            {
                "request": request,
                "event": event,
            },
        )

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
    @app.post("/package/{name}/retry")
    async def retry_with_recipe(name: str, request: Request):
        # Basic action: no-op placeholder returning a message; actual build orchestration is out of scope for UI.
        return JSONResponse({"detail": f"Retry with recipe requested for {name}. Trigger via CLI/automation."})
