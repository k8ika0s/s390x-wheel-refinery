from pathlib import Path

from s390x_wheel_refinery.history import BuildHistory


def test_failures_over_time(tmp_path: Path):
    hist = BuildHistory(tmp_path / "history.db")
    hist.record_event(run_id="r", name="pkg", version="1", python_tag="cp311", platform_tag="x", status="failed")
    assert hist.failures_over_time(limit=5)


def test_variant_history(tmp_path: Path):
    hist = BuildHistory(tmp_path / "history.db")
    hist.record_event(
        run_id="r",
        name="pkg",
        version="1",
        python_tag="cp311",
        platform_tag="x",
        status="built",
        metadata={"variant": "default"},
    )
    assert hist.variant_history("pkg", limit=5)
