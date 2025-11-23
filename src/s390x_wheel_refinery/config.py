from __future__ import annotations

import json
import re
import tomllib
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Dict, List, Optional


class UpgradeStrategy(str, Enum):
    """Controls how dependency versions are selected."""

    PINNED = "pinned"
    # Future strategies may allow upgrades; keep the enum extensible.
    EAGER = "eager"


@dataclass
class IndexSettings:
    index_url: Optional[str] = None
    extra_index_urls: List[str] = field(default_factory=list)
    trusted_hosts: List[str] = field(default_factory=list)


@dataclass
class PackageOverride:
    system_packages: List[str] = field(default_factory=list)
    system_recipe: List[str] = field(default_factory=list)
    env: Dict[str, str] = field(default_factory=dict)
    notes: Optional[str] = None


@dataclass
class RefineryConfig:
    target_python: str
    target_platform_tag: str = "manylinux2014_s390x"
    upgrade_strategy: UpgradeStrategy = UpgradeStrategy.PINNED
    index: IndexSettings = field(default_factory=IndexSettings)
    overrides: Dict[str, PackageOverride] = field(default_factory=dict)
    allow_system_recipes: bool = True
    dry_run_recipes: bool = False
    max_attempts: int = 3
    attempt_timeout: int = 900  # seconds
    attempt_backoff_base: int = 5  # seconds
    attempt_backoff_max: int = 60  # seconds
    container_image: Optional[str] = None
    container_engine: str = "docker"
    container_preset: Optional[str] = None
    auto_apply_suggestions: bool = False
    fallback_latest: bool = False
    container_cpu: Optional[str] = None
    container_memory: Optional[str] = None

    @property
    def python_tag(self) -> str:
        """Return PEP 425 python tag (e.g. cp311) from the configured version."""
        return python_tag_from_version(self.target_python)


def python_tag_from_version(version: str) -> str:
    match = re.fullmatch(r"(\d+)\.(\d+)", version.strip())
    if not match:
        raise ValueError(f"Invalid Python version '{version}'. Use format like 3.11.")
    major, minor = match.groups()
    return f"cp{major}{minor}"


def load_config(path: Optional[Path]) -> Dict:
    """Load a config file from TOML or JSON."""
    if path is None:
        return {}
    if not path.exists():
        raise FileNotFoundError(f"Config file {path} was not found.")
    if path.suffix in {".toml", ".tml"}:
        return tomllib.loads(path.read_text())
    if path.suffix in {".json"}:
        return json.loads(path.read_text())
    raise ValueError(f"Unsupported config format for {path}. Use TOML or JSON.")


def build_config(
    *,
    target_python: str,
    target_platform_tag: str = "manylinux2014_s390x",
    upgrade_strategy: str = UpgradeStrategy.PINNED.value,
    index_url: Optional[str] = None,
    extra_index_urls: Optional[List[str]] = None,
    trusted_hosts: Optional[List[str]] = None,
    config_file: Optional[Path] = None,
    allow_system_recipes: Optional[bool] = None,
    dry_run_recipes: Optional[bool] = None,
    max_attempts: Optional[int] = None,
    attempt_timeout: Optional[int] = None,
    attempt_backoff_base: Optional[int] = None,
    attempt_backoff_max: Optional[int] = None,
    container_image: Optional[str] = None,
    container_engine: Optional[str] = None,
    container_preset: Optional[str] = None,
    auto_apply_suggestions: Optional[bool] = None,
    fallback_latest: Optional[bool] = None,
    container_cpu: Optional[str] = None,
    container_memory: Optional[str] = None,
) -> RefineryConfig:
    """Merge CLI inputs with any file-based configuration."""
    file_data = load_config(config_file)
    cfg = file_data.get("refinery", {}) if isinstance(file_data, dict) else {}

    index_section = cfg.get("index", {})
    index = IndexSettings(
        index_url=index_url or index_section.get("index_url"),
        extra_index_urls=extra_index_urls or index_section.get("extra_index_urls", []),
        trusted_hosts=trusted_hosts or index_section.get("trusted_hosts", []),
    )

    overrides_section = cfg.get("overrides", {})
    overrides = {
        name: PackageOverride(
            system_packages=override.get("system_packages", []),
            system_recipe=override.get("system_recipe", []),
            env=override.get("env", {}),
            notes=override.get("notes"),
        )
        for name, override in overrides_section.items()
    }

    final_python = target_python or cfg.get("target_python")
    if not final_python:
        raise ValueError("A target Python version is required (e.g. 3.11).")

    strategy_value = upgrade_strategy or cfg.get("upgrade_strategy", UpgradeStrategy.PINNED.value)
    strategy = UpgradeStrategy(strategy_value)

    return RefineryConfig(
        target_python=final_python,
        target_platform_tag=target_platform_tag or cfg.get("target_platform_tag", "manylinux2014_s390x"),
        upgrade_strategy=strategy,
        index=index,
        overrides=overrides,
        allow_system_recipes=_maybe_bool(allow_system_recipes, cfg.get("allow_system_recipes", True)),
        dry_run_recipes=_maybe_bool(dry_run_recipes, cfg.get("dry_run_recipes", False)),
        max_attempts=max_attempts or cfg.get("max_attempts", 3),
        attempt_timeout=attempt_timeout or cfg.get("attempt_timeout", 900),
        attempt_backoff_base=attempt_backoff_base or cfg.get("attempt_backoff_base", 5),
        attempt_backoff_max=attempt_backoff_max or cfg.get("attempt_backoff_max", 60),
        container_image=container_image or cfg.get("container_image"),
        container_engine=container_engine or cfg.get("container_engine", "docker"),
        container_preset=container_preset or cfg.get("container_preset"),
        auto_apply_suggestions=_maybe_bool(auto_apply_suggestions, cfg.get("auto_apply_suggestions", False)),
        fallback_latest=_maybe_bool(fallback_latest, cfg.get("fallback_latest", False)),
        container_cpu=container_cpu or cfg.get("container_cpu"),
        container_memory=container_memory or cfg.get("container_memory"),
    )


def _maybe_bool(cli_value: Optional[bool], cfg_value: Optional[bool]) -> bool:
    if cli_value is not None:
        return cli_value
    return bool(cfg_value) if cfg_value is not None else False
