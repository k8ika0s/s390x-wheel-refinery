from pathlib import Path

from s390x_wheel_refinery.config import build_config
from s390x_wheel_refinery.resolver import build_plan
from s390x_wheel_refinery.scanner import scan_wheels


def test_scan_and_resolve_reuse_vs_build(tmp_path: Path):
    # Reusable pure wheel
    from tests.conftest import write_dummy_wheel

    reusable = write_dummy_wheel(tmp_path, "purepkg", "1.0.0", python_tag="py3", abi_tag="none", platform_tag="any")
    # Incompatible wheel triggers rebuild
    rebuild = write_dummy_wheel(
        tmp_path,
        "nativepkg",
        "1.0.0",
        python_tag="cp311",
        abi_tag="cp311",
        platform_tag="manylinux2014_x86_64",
        requires=["dep==1.0.0"],
    )

    wheels = scan_wheels(tmp_path)
    cfg = build_config(target_python="3.11", target_platform_tag="manylinux2014_s390x")
    plan = build_plan(wheels, cfg)

    assert any(r.name == "purepkg" for r in plan.reusable)
    assert any(job.name == "nativepkg" for job in plan.to_build)
    # Missing pinned dependency should be planned
    assert any(job.name.lower() == "dep" for job in plan.to_build)
