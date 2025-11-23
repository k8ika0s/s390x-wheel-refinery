from pathlib import Path

from s390x_wheel_refinery.history import BuildHistory
from s390x_wheel_refinery.models import BuildJob
from s390x_wheel_refinery.scheduler import schedule_jobs


def test_schedule_shortest_first(tmp_path: Path):
    history = BuildHistory(tmp_path / "history.db")
    # Record events with durations
    history.record_event(
        run_id="run",
        name="fastpkg",
        version="1.0",
        python_tag="cp311",
        platform_tag="manylinux2014_s390x",
        status="built",
        metadata={"duration_seconds": 1},
    )
    history.record_event(
        run_id="run",
        name="slowpkg",
        version="1.0",
        python_tag="cp311",
        platform_tag="manylinux2014_s390x",
        status="built",
        metadata={"duration_seconds": 10},
    )

    jobs = [
        BuildJob(name="fastpkg", version="1.0", python_tag="cp311", platform_tag="manylinux2014_s390x", source_spec="", reason="", depth=0),
        BuildJob(name="slowpkg", version="1.0", python_tag="cp311", platform_tag="manylinux2014_s390x", source_spec="", reason="", depth=1),
    ]
    ordered = schedule_jobs(jobs, history, strategy="shortest-first")
    # Depth is primary, then avg duration
    assert ordered[0].name == "fastpkg"
