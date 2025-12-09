from pathlib import Path

import pytest

pytest.importorskip("fastapi")
from fastapi.testclient import TestClient  # type: ignore  # noqa: E402

from s390x_wheel_refinery.history import BuildHistory  # noqa: E402
from s390x_wheel_refinery.web import create_app  # noqa: E402


def test_web_endpoints(tmp_path: Path):
    history = BuildHistory(tmp_path / "history.db")
    history.record_event(
        run_id="run",
        name="pkg",
        version="1.0",
        python_tag="cp311",
        platform_tag="manylinux2014_s390x",
        status="built",
        source_spec="pkg==1.0",
        detail="ok",
        metadata={"duration_seconds": 1},
    )
    app = create_app(history)
    client = TestClient(app)
    resp = client.get("/api/top-failures")
    assert resp.status_code == 200
    resp = client.get("/api/top-slowest")
    assert resp.status_code == 200
    resp = client.get("/api/queue")
    assert resp.status_code == 200
    payload = resp.json()
    assert payload["length"] == 0
    assert payload["items"] == []


def test_worker_trigger(monkeypatch, tmp_path: Path):
    input_dir = tmp_path / "in"
    output_dir = tmp_path / "out"
    cache_dir = tmp_path / "cache"
    for d in (input_dir, output_dir, cache_dir):
        d.mkdir()

    monkeypatch.setenv("WORKER_INPUT_DIR", str(input_dir))
    monkeypatch.setenv("WORKER_OUTPUT_DIR", str(output_dir))
    monkeypatch.setenv("WORKER_CACHE_DIR", str(cache_dir))
    monkeypatch.setenv("WORKER_PYTHON", "3.11")

    called = {}

    def fake_process_queue(**kwargs):
        called["run"] = kwargs

    monkeypatch.setattr("s390x_wheel_refinery.web.process_queue", lambda **kwargs: fake_process_queue(**kwargs))

    history = BuildHistory(tmp_path / "history.db")
    app = create_app(history)
    client = TestClient(app)
    resp = client.post("/api/worker/trigger")
    assert resp.status_code == 200
    assert "run" in called


def test_worker_trigger_webhook(monkeypatch, tmp_path: Path):
    monkeypatch.setenv("WORKER_WEBHOOK_URL", "http://worker/trigger")
    monkeypatch.setenv("WORKER_TOKEN", "secret")
    called = {}

    async def fake_trigger(url, token):
        called["url"] = url
        called["token"] = token
        return True, "ok"

    monkeypatch.setattr("s390x_wheel_refinery.web._trigger_worker_webhook", fake_trigger)

    history = BuildHistory(tmp_path / "history.db")
    app = create_app(history)
    client = TestClient(app)
    resp = client.post("/api/worker/trigger", headers={"X-Worker-Token": "secret"})
    assert resp.status_code == 200
    assert called["url"] == "http://worker/trigger"
    assert called["token"] == "secret"


def test_set_token_cookie(monkeypatch, tmp_path: Path):
    monkeypatch.setenv("WORKER_TOKEN", "secret")
    history = BuildHistory(tmp_path / "history.db")
    app = create_app(history)
    client = TestClient(app)
    resp = client.post("/api/session/token?token=secret")
    assert resp.status_code == 200
    assert any(c.startswith("worker_token=") for c in resp.headers.get("set-cookie", "").split(";"))
