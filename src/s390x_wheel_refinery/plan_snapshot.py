from __future__ import annotations

import json
from pathlib import Path
from typing import Iterable

from .models import Plan


def _nodes_from_plan(plan: Plan, python_tag: str, platform_tag: str):
    nodes = []
    for reuse in plan.reusable:
        nodes.append(
            {
                "name": reuse.name,
                "version": reuse.version,
                "python_tag": python_tag,
                "platform_tag": platform_tag,
                "action": "reuse",
            }
        )
    for job in plan.to_build:
        nodes.append(
            {
                "name": job.name,
                "version": job.version,
                "python_tag": job.python_tag or python_tag,
                "platform_tag": job.platform_tag or platform_tag,
                "action": "build",
            }
        )
    return nodes


def write_plan_snapshot(plan: Plan, path: Path, *, run_id: str, python_tag: str, platform_tag: str) -> Path:
    """Serialize a plan snapshot to JSON for the control plane."""
    nodes = _nodes_from_plan(plan, python_tag, platform_tag)
    payload = {"run_id": run_id, "plan": nodes}
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2))
    return path
