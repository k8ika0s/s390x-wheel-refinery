import argparse

from src.s390x_wheel_refinery import cli
from src.s390x_wheel_refinery.models import BuildJob


def test_filter_jobs_for_only_by_name():
    jobs = [
        BuildJob(name="pkgA", version="1.0.0", python_tag="cp311", platform_tag="manylinux", source_spec="", reason=""),
        BuildJob(name="pkgB", version="2.0.0", python_tag="cp311", platform_tag="manylinux", source_spec="", reason=""),
    ]
    filtered = cli._filter_jobs_for_only(jobs, ["pkga"])
    assert len(filtered) == 1
    assert filtered[0].name == "pkgA"


def test_filter_jobs_for_only_by_name_and_version():
    jobs = [
        BuildJob(name="pkgA", version="1.0.0", python_tag="cp311", platform_tag="manylinux", source_spec="", reason=""),
        BuildJob(name="pkgA", version="2.0.0", python_tag="cp311", platform_tag="manylinux", source_spec="", reason=""),
    ]
    filtered = cli._filter_jobs_for_only(jobs, ["pkgA==2.0.0"])
    assert len(filtered) == 1
    assert filtered[0].version == "2.0.0"


def test_parse_run_args_supports_only():
    ns = cli._parse_run_args(
        [
            "--input",
            "/in",
            "--output",
            "/out",
            "--cache",
            "/cache",
            "--python",
            "3.11",
            "--only",
            "pkgA",
            "--only",
            "pkgB==1.0",
        ]
    )
    assert isinstance(ns, argparse.Namespace)
    assert ns.only == ["pkgA", "pkgB==1.0"]
