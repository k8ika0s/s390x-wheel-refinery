from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable, List

from .models import BuildJob
from .history import BuildHistory


@dataclass
class ScheduledJob:
    job: BuildJob
    priority: float


def schedule_jobs(jobs: Iterable[BuildJob], history: BuildHistory, strategy: str = "shortest-first") -> List[BuildJob]:
    if strategy == "shortest-first":
        scheduled = []
        for job in jobs:
            avg = _avg_duration(history, job.name)
            depth_priority = job.depth if hasattr(job, "depth") else 0
            duration_priority = avg if avg is not None else float("inf")
            cpu_priority = job.resource_cpu if job.resource_cpu is not None else 0
            mem_priority = job.resource_mem if job.resource_mem is not None else 0
            priority = (depth_priority, duration_priority, cpu_priority, mem_priority)
            scheduled.append(ScheduledJob(job=job, priority=priority))
        scheduled.sort(key=lambda j: j.priority)
        return [sj.job for sj in scheduled]
    return list(jobs)


def _avg_duration(history: BuildHistory, name: str):
    summary = history.package_summary(name)
    return summary.avg_duration
