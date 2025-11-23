from pathlib import Path

import pytest

fastapi = pytest.importorskip("fastapi")
from fastapi.testclient import TestClient  # type: ignore

from s390x_wheel_refinery.history import BuildHistory
from s390x_wheel_refinery.web import create_app


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
