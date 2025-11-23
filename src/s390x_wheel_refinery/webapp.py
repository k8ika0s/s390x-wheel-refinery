from __future__ import annotations

import os
from pathlib import Path

from .history import BuildHistory
from .web import create_app


def _history_path() -> Path:
    env_path = os.environ.get("HISTORY_DB")
    if env_path:
        return Path(env_path)
    return Path("/cache/history.db")


history = BuildHistory(_history_path())
app = create_app(history)
