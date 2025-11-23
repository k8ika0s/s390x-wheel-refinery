from __future__ import annotations

import json
from dataclasses import dataclass, asdict
from pathlib import Path
from typing import List


@dataclass
class RetryRequest:
    package: str
    version: str
    python_tag: str
    platform_tag: str
    recipes: List[str]


class RetryQueue:
    def __init__(self, path: Path):
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        if not self.path.exists():
            self._save([])

    def add(self, request: RetryRequest) -> None:
        data = self._load()
        data.append(asdict(request))
        self._save(data)

    def pop_all(self) -> List[RetryRequest]:
        data = self._load()
        self._save([])
        return [RetryRequest(**item) for item in data]

    def _load(self) -> list:
        try:
            return json.loads(self.path.read_text())
        except Exception:
            return []

    def _save(self, data: list) -> None:
        self.path.write_text(json.dumps(data, indent=2))
