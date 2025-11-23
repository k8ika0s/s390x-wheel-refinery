from pathlib import Path

from conftest import write_dummy_wheel
from s390x_wheel_refinery.config import build_config
from s390x_wheel_refinery.resolver import build_plan
from s390x_wheel_refinery.scanner import scan_wheels


class DummyIndex:
    def versions(self, project: str):
        return set()


def test_dependency_expansion_depth_and_parent(tmp_path: Path):
    write_dummy_wheel(tmp_path, "parentpkg", "1.0.0", python_tag="cp311", abi_tag="cp311", platform_tag="manylinux2014_x86_64", requires=["childdep"])
    wheels = scan_wheels(tmp_path)
    cfg = build_config(target_python="3.11", target_platform_tag="manylinux2014_s390x")
    plan = build_plan(wheels, cfg, index_client=DummyIndex())
    deps = [job for job in plan.to_build if job.reason.startswith("dependency expansion")]
    assert deps
    assert all(job.depth >= 1 for job in deps)
