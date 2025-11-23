from __future__ import annotations

import re
import yaml
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional


@dataclass
class Hint:
    pattern: str
    packages: Dict[str, List[str]]  # keyed by distro (dnf/apt)


class HintCatalog:
    def __init__(self, path: Path):
        self.hints: List[Hint] = []
        if path.exists():
            data = yaml.safe_load(path.read_text())
            for entry in data.get("errors", []):
                self.hints.append(Hint(pattern=entry["pattern"], packages=entry.get("packages", {})))

    def match(self, output: str) -> Optional[Hint]:
        for hint in self.hints:
            if re.search(hint.pattern, output):
                return hint
        return None
