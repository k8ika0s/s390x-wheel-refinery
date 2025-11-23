from pathlib import Path

from s390x_wheel_refinery.history import BuildHistory


def test_status_counts_recent(tmp_path: Path):
    history = BuildHistory(tmp_path / "history.db")
    history.record_event(run_id="r", name="a", version="1", python_tag="cp311", platform_tag="x", status="built")
    history.record_event(run_id="r", name="b", version="1", python_tag="cp311", platform_tag="x", status="failed")
    counts = history.status_counts_recent(limit=10)
    assert counts["built"] == 1
    assert counts["failed"] == 1
