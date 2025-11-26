from pathlib import Path

from s390x_wheel_refinery.queue import RetryQueue, RetryRequest
from s390x_wheel_refinery.cli import main


def test_retry_queue_roundtrip(tmp_path: Path):
    qpath = tmp_path / "q.json"
    queue = RetryQueue(qpath)
    queue.add(RetryRequest(package="pkg", version="1", python_tag="cp311", platform_tag="x", recipes=["dnf install foo"]))
    items = queue.pop_all()
    assert len(items) == 1
    assert items[0].package == "pkg"
    # Queue should now be empty
    assert queue.pop_all() == []


def test_cli_queue(tmp_path: Path):
    qpath = tmp_path / "q.json"
    queue = RetryQueue(qpath)
    queue.add(RetryRequest(package="pkg", version="1", python_tag="cp311", platform_tag="x", recipes=[]))
    ret = main(["queue", "--queue-path", str(qpath)])
    assert ret == 0
