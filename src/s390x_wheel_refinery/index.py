from __future__ import annotations

import logging
import subprocess
import sys
from functools import lru_cache
from typing import Iterable, Set

from packaging.version import InvalidVersion, Version

from .config import IndexSettings

LOG = logging.getLogger(__name__)


class IndexClient:
    """Lightweight wrapper around `pip index versions` to discover available releases."""

    def __init__(self, index: IndexSettings):
        self.index = index

    @lru_cache(maxsize=None)
    def versions(self, project: str) -> Set[Version]:
        cmd = [
            sys.executable,
            "-m",
            "pip",
            "index",
            "versions",
            project,
            *self._index_args(),
        ]
        try:
            result = subprocess.run(cmd, check=True, capture_output=True, text=True)
        except subprocess.CalledProcessError as exc:
            LOG.debug("Failed to query versions for %s: %s", project, exc)
            return set()

        versions_line = None
        for line in result.stdout.splitlines():
            if "Available versions" in line:
                versions_line = line
                break

        if not versions_line:
            LOG.debug("No versions found in pip output for %s", project)
            return set()

        _, _, version_str = versions_line.partition(":")
        raw_versions = [v.strip() for v in version_str.split(",") if v.strip()]
        parsed: Set[Version] = set()
        for raw in raw_versions:
            try:
                parsed.add(Version(raw))
            except InvalidVersion:
                continue
        return parsed

    def _index_args(self) -> Iterable[str]:
        args = []
        if self.index.index_url:
            args.extend(["--index-url", self.index.index_url])
        for extra in self.index.extra_index_urls:
            args.extend(["--extra-index-url", extra])
        for host in self.index.trusted_hosts:
            args.extend(["--trusted-host", host])
        return args
