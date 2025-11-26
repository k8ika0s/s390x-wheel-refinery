from __future__ import annotations

import logging
from pathlib import Path
from typing import Optional

from .builder import WheelBuilder
from .config import PackageOverride, build_config
from .history import BuildHistory
from .queue import RetryQueue
from .resolver import build_plan
from .scanner import scan_wheels

LOG = logging.getLogger(__name__)


def process_queue(
    input_dir: Path,
    output_dir: Path,
    cache_dir: Path,
    *,
    history_path: Path,
    python_version: str,
    platform_tag: str,
    container_image: Optional[str] = None,
    container_preset: Optional[str] = None,
) -> None:
    queue = RetryQueue(history_path.parent / "retry_queue.json")
    requests = queue.pop_all()
    if not requests:
        LOG.info("No retry requests in queue.")
        return

    history = BuildHistory(history_path)
    cfg = build_config(
        target_python=python_version,
        target_platform_tag=platform_tag,
        container_image=container_image,
        container_preset=container_preset,
    )
    builder = WheelBuilder(cache_dir, output_dir, cfg, history=history, run_id="worker")
    builder.ensure_ready()

    wheels = scan_wheels(input_dir)
    plan = build_plan(wheels, cfg)
    for req in requests:
        LOG.info("Processing retry request for %s", req.package)
        matched = False
        for job in plan.to_build:
            if job.name.lower() != req.package.lower():
                continue
            if req.version not in ("latest", "", None) and job.version != req.version:
                continue
            matched = True
            override = _get_or_create_override(builder, job.name)
            for step in req.recipes:
                if step not in override.system_recipe:
                    override.system_recipe.append(step)
            builder.build_job(job)
        if not matched:
            LOG.warning("No matching job found in plan for %s (version=%s)", req.package, req.version)


def _get_or_create_override(builder: WheelBuilder, job_name: str) -> PackageOverride:
    override = builder.config.overrides.get(job_name) or builder.config.overrides.get(job_name.lower())
    if override is None:
        override = PackageOverride()
        builder.config.overrides[job_name] = override
    return override
