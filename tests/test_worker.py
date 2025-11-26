from pathlib import Path
from types import SimpleNamespace

import s390x_wheel_refinery.worker as worker
from s390x_wheel_refinery.cli import main
from s390x_wheel_refinery.queue import RetryQueue, RetryRequest


def test_process_queue_applies_recipes(monkeypatch, tmp_path: Path):
    cache = tmp_path / "cache"
    output = tmp_path / "out"
    input_dir = tmp_path / "in"
    cache.mkdir()
    output.mkdir()
    input_dir.mkdir()

    queue = RetryQueue(cache / "retry_queue.json")
    queue.add(
        RetryRequest(
            package="pkg",
            version="1.0.0",
            python_tag="cp311",
            platform_tag="manylinux2014_s390x",
            recipes=["dnf install zlib-devel"],
        )
    )

    job = SimpleNamespace(
        name="pkg",
        version="1.0.0",
        python_tag="cp311",
        platform_tag="manylinux2014_s390x",
        source_spec="pkg==1.0.0",
    )
    plan = SimpleNamespace(to_build=[job])
    built = {}

    class DummyBuilder:
        def __init__(self, cache_dir, output_dir, cfg, history=None, run_id=None, index_client=None):
            self.config = cfg
            built["config"] = cfg

        def ensure_ready(self):
            built["ready"] = True

        def build_job(self, job_arg):
            built.setdefault("calls", []).append(job_arg)

    monkeypatch.setattr(worker, "WheelBuilder", DummyBuilder)
    monkeypatch.setattr(worker, "scan_wheels", lambda path: [])
    monkeypatch.setattr(worker, "build_plan", lambda wheels, cfg: plan)

    worker.process_queue(
        input_dir=input_dir,
        output_dir=output,
        cache_dir=cache,
        history_path=cache / "history.db",
        python_version="3.11",
        platform_tag="manylinux2014_s390x",
    )

    override = built["config"].overrides["pkg"]
    assert "dnf install zlib-devel" in override.system_recipe
    assert built["calls"] == [job]
    assert len(queue) == 0


def test_cli_dispatches_worker(monkeypatch, tmp_path: Path):
    captured = {}

    def fake_process_queue(**kwargs):
        captured.update(kwargs)

    monkeypatch.setattr("s390x_wheel_refinery.cli.process_queue", fake_process_queue)

    ret = main(
        [
            "worker",
            "--input",
            str(tmp_path / "in"),
            "--output",
            str(tmp_path / "out"),
            "--cache",
            str(tmp_path / "cache"),
            "--python",
            "3.11",
        ]
    )

    assert ret == 0
    assert captured["python_version"] == "3.11"
    assert captured["platform_tag"] == "manylinux2014_s390x"
    assert captured["history_path"] == (tmp_path / "cache" / "history.db")
