from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Optional


@dataclass
class BuildJob:
    name: str
    version: str
    python_tag: str
    platform_tag: str
    source_spec: str
    reason: str
    depth: int = 0
    parents: list[str] = field(default_factory=list)


@dataclass
class Plan:
    reusable: List["ReusableWheel"] = field(default_factory=list)
    to_build: List[BuildJob] = field(default_factory=list)
    missing_requirements: List[str] = field(default_factory=list)
    dependency_expansions: List[BuildJob] = field(default_factory=list)


@dataclass
class ReusableWheel:
    name: str
    version: str
    path: str


@dataclass
class ManifestEntry:
    name: str
    version: str
    status: str  # e.g. reused, built, cached, missing, failed
    path: Optional[str] = None
    detail: Optional[str] = None
    metadata: Optional[dict] = None


@dataclass
class Manifest:
    python_tag: str
    platform_tag: str
    entries: List[ManifestEntry]
