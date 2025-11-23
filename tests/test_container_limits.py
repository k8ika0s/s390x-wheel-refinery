from s390x_wheel_refinery.builder import WheelBuilder
from s390x_wheel_refinery.config import build_config


def test_containerized_command_respects_job_hints(tmp_path):
    cfg = build_config(target_python="3.11")
    builder = WheelBuilder(tmp_path, tmp_path, cfg)
    builder._container_image = "dummy"
    cmd = builder._containerized_command(["echo", "hi"], env={}, workdir=None, cpu=2, mem="4g")
    assert "--cpus" in cmd and "2" in cmd
    assert "--memory" in cmd and "4g" in cmd
