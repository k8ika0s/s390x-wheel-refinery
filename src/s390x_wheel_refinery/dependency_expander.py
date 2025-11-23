from __future__ import annotations

from typing import Iterable, List, Set

from packaging.requirements import Requirement

from .models import BuildJob
from .scanner import WheelInfo


def missing_python_deps(wheels: Iterable[WheelInfo], planned: Iterable[BuildJob]) -> List[str]:
    planned_names: Set[str] = {job.name.lower() for job in planned}
    wheel_names: Set[str] = {wheel.name.lower() for wheel in wheels}
    missing: Set[str] = set()
    for wheel in wheels:
        for req in wheel.requires_dist:
            name = req.name.lower()
            if name not in wheel_names and name not in planned_names:
                missing.add(name)
    return sorted(missing)


def build_jobs_for_missing(
    missing: Iterable[str],
    python_tag: str,
    platform_tag: str,
    reason: str = "dependency expansion",
    max_count: int = 5,
    depth: int = 0,
    parent: str | None = None,
) -> List[BuildJob]:
    jobs: List[BuildJob] = []
    for name in missing:
        if len(jobs) >= max_count:
            break
        jobs.append(
            BuildJob(
                name=name,
                version="latest",
                python_tag=python_tag,
                platform_tag=platform_tag,
                source_spec=name,
                reason=reason,
                depth=depth,
                parents=[parent] if parent else [],
            )
        )
    return jobs
