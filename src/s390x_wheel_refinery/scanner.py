from __future__ import annotations

import logging
import zipfile
from dataclasses import dataclass
from email.parser import Parser
from pathlib import Path
from typing import Iterable, List, Optional, Set

from packaging.requirements import Requirement
from packaging.tags import Tag
from packaging.utils import InvalidWheelFilename, parse_wheel_filename

LOG = logging.getLogger(__name__)


@dataclass
class WheelInfo:
    name: str
    version: str
    filename: str
    path: Path
    tags: Set[Tag]
    requires_dist: List[Requirement]
    summary: Optional[str] = None

    @property
    def is_pure_python(self) -> bool:
        return any(tag.platform == "any" for tag in self.tags)

    def supports(self, python_tag: str, platform_tag: str) -> bool:
        """Check if the wheel works for the target Python and platform."""
        for tag in self.tags:
            python_ok = tag.python in {python_tag, "py3"} or tag.python.startswith("py3")
            platform_ok = tag.platform == "any" or tag.platform == platform_tag or (
                tag.platform.endswith("_s390x") and platform_tag.endswith("_s390x")
            )
            if python_ok and platform_ok:
                return True
        return False


def parse_metadata(metadata_text: str) -> dict:
    message = Parser().parsestr(metadata_text)
    requires = message.get_all("Requires-Dist") or []
    summary = message.get("Summary")
    return {"requires": [Requirement(req) for req in requires], "summary": summary}


def read_wheel_metadata(path: Path) -> WheelInfo:
    try:
        name, version, build, tags = parse_wheel_filename(path.name)
    except InvalidWheelFilename as exc:
        raise ValueError(f"Invalid wheel filename: {path}") from exc

    requires: List[Requirement] = []
    summary: Optional[str] = None
    with zipfile.ZipFile(path) as zf:
        metadata_path = _metadata_path(zf.namelist())
        if metadata_path:
            metadata_text = zf.read(metadata_path).decode("utf-8", errors="replace")
            parsed = parse_metadata(metadata_text)
            requires = parsed["requires"]
            summary = parsed["summary"]
        else:
            LOG.warning("Wheel %s missing METADATA file.", path)

    return WheelInfo(
        name=name,
        version=str(version),
        filename=path.name,
        path=path,
        tags=set(tags),
        requires_dist=requires,
        summary=summary,
    )


def _metadata_path(entries: Iterable[str]) -> Optional[str]:
    for entry in entries:
        if entry.endswith("METADATA") and ".dist-info/" in entry:
            return entry
    return None


def scan_wheels(directory: Path) -> List[WheelInfo]:
    wheels: List[WheelInfo] = []
    for wheel_path in sorted(directory.glob("*.whl")):
        try:
            wheels.append(read_wheel_metadata(wheel_path))
        except Exception as exc:
            LOG.error("Failed to read %s: %s", wheel_path, exc)
    return wheels
