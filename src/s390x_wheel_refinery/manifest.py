from __future__ import annotations

import json
from pathlib import Path

from .models import Manifest


def write_manifest(manifest: Manifest, path: Path) -> None:
    payload = {
        "python_tag": manifest.python_tag,
        "platform_tag": manifest.platform_tag,
        "entries": [
            {
                "name": entry.name,
                "version": entry.version,
                "status": entry.status,
                "path": entry.path,
                "detail": entry.detail,
                "metadata": entry.metadata,
            }
            for entry in manifest.entries
        ],
    }
    path.write_text(json.dumps(payload, indent=2))
