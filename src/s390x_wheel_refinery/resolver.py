from __future__ import annotations

import logging
from collections import defaultdict
from typing import Dict, Iterable, List, Optional, Set

from packaging.requirements import Requirement
from packaging.version import InvalidVersion, Version

from .config import RefineryConfig, UpgradeStrategy
from .index import IndexClient
from .dependency_expander import missing_python_deps, build_jobs_for_missing
from .models import BuildJob, Plan, ReusableWheel
from .scanner import WheelInfo

LOG = logging.getLogger(__name__)


def build_plan(
    wheels: List[WheelInfo],
    config: RefineryConfig,
    *,
    index_client: Optional[IndexClient] = None,
    max_dep_depth: int = 2,
    max_dep_attempts: int = 1,
) -> Plan:
    plan = Plan()
    reusable_versions: Dict[str, Set[Version]] = defaultdict(set)
    planned_versions: Dict[str, Set[Version]] = defaultdict(set)
    all_versions: Dict[str, Set[Version]] = defaultdict(set)

    # First pass: classify existing wheels and collect known versions.
    for wheel in wheels:
        version_obj = _parse_version(wheel.version)
        normalized = wheel.name.lower()
        all_versions[normalized].add(version_obj)

        if wheel.is_pure_python or wheel.supports(config.python_tag, config.target_platform_tag):
            plan.reusable.append(ReusableWheel(name=wheel.name, version=wheel.version, path=str(wheel.path)))
            reusable_versions[normalized].add(version_obj)
            continue

        if _already_planned_version(wheel.name, wheel.version, plan.to_build):
            planned_versions[normalized].add(version_obj)
            continue

        plan.to_build.append(
            BuildJob(
                name=wheel.name,
                version=wheel.version,
                python_tag=config.python_tag,
                platform_tag=config.target_platform_tag,
                source_spec=f"{wheel.name}=={wheel.version}",
                reason="incompatible wheel",
            )
        )
        planned_versions[normalized].add(version_obj)

    # Second pass: resolve missing dependencies.
    requirements = _collect_requirements(wheels)
    for req in requirements:
        normalized = req.name.lower()
        satisfied_versions = _merged_versions(reusable_versions.get(normalized), planned_versions.get(normalized))
        if _satisfies(req, satisfied_versions):
            continue

        pinned = _pinned_version(req)
        if pinned:
            LOG.info("Planning build for missing pinned dependency %s", req)
            if not _already_planned_version(normalized, pinned, plan.to_build):
                plan.to_build.append(
                    BuildJob(
                        name=req.name,
                        version=pinned,
                        python_tag=config.python_tag,
                        platform_tag=config.target_platform_tag,
                        source_spec=f"{req.name}=={pinned}",
                        reason="missing dependency",
                    )
                )
                planned_versions[normalized].add(_parse_version(pinned))
            continue

        candidate_versions = set(all_versions.get(normalized, set()))
        if config.upgrade_strategy == UpgradeStrategy.EAGER and index_client:
            remote_versions = index_client.versions(req.name)
            candidate_versions.update(remote_versions)
        elif config.fallback_latest and index_client:
            remote_versions = index_client.versions(req.name)
            candidate_versions.update(remote_versions)

        candidate = _best_candidate(req, candidate_versions)
        if candidate and not _already_planned_version(normalized, str(candidate), plan.to_build):
            LOG.info("Planning build for %s via best available version %s", req.name, candidate)
            plan.to_build.append(
                BuildJob(
                    name=req.name,
                    version=str(candidate),
                    python_tag=config.python_tag,
                    platform_tag=config.target_platform_tag,
                    source_spec=f"{req.name}=={candidate}",
                    reason="missing compatible wheel for requirement",
                )
            )
            planned_versions[normalized].add(candidate)
            continue

        plan.missing_requirements.append(str(req))

    # Dependency expansion: recursive bounded
    _expand_dependencies(plan, wheels, config, index_client, max_dep_depth=max_dep_depth, max_dep_attempts=max_dep_attempts)

    return plan


def _expand_dependencies(
    plan: Plan,
    wheels: List[WheelInfo],
    config: RefineryConfig,
    index_client: Optional[IndexClient],
    *,
    max_dep_depth: int,
    max_dep_attempts: int,
) -> None:
    if not index_client or max_dep_depth <= 0:
        return
    # Only expand deps for jobs within depth budget
    to_consider = [job for job in plan.to_build if job.depth < max_dep_depth]
    if not to_consider:
        return
    missing = missing_python_deps(wheels, to_consider)
    if not missing:
        return
    expansion_jobs = build_jobs_for_missing(
        missing,
        config.python_tag,
        config.target_platform_tag,
        reason="dependency expansion",
        max_count=max_dep_attempts,
        depth=1,
    )
    for exp in expansion_jobs:
        exp.parents = list({job.name for job in to_consider})
        plan.dependency_expansions.append(exp)
        plan.to_build.append(exp)


def _already_planned_version(name: str, version: str, jobs: Iterable[BuildJob]) -> bool:
    target = name.lower()
    return any(job.name.lower() == target and job.version == version for job in jobs)


def _collect_requirements(wheels: Iterable[WheelInfo]) -> List[Requirement]:
    requirements: List[Requirement] = []
    for wheel in wheels:
        requirements.extend(wheel.requires_dist)
    return requirements


def _parse_version(version: str) -> Version:
    try:
        return Version(str(version))
    except InvalidVersion as exc:
        raise ValueError(f"Invalid version string: {version}") from exc


def _merged_versions(primary: Set[Version] | None, secondary: Set[Version] | None) -> Set[Version]:
    merged: Set[Version] = set()
    if primary:
        merged.update(primary)
    if secondary:
        merged.update(secondary)
    return merged


def _satisfies(req: Requirement, versions: Set[Version]) -> bool:
    if not versions:
        return False
    if not req.specifier:
        return True
    return any(req.specifier.contains(v, prereleases=True) for v in versions)


def _best_candidate(req: Requirement, versions: Set[Version]) -> Version | None:
    best: Version | None = None
    for version in versions:
        if req.specifier and not req.specifier.contains(version, prereleases=True):
            continue
        if best is None or version > best:
            best = version
    return best


def _pinned_version(req: Requirement) -> str | None:
    specifiers = list(req.specifier)
    if len(specifiers) != 1:
        return None
    spec = specifiers[0]
    if spec.operator == "==" and spec.version:
        return spec.version
    if spec.operator == "===" and spec.version:
        return spec.version
    return None
