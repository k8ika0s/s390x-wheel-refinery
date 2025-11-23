from __future__ import annotations

import json
import sqlite3
import threading
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional


class BuildHistory:
    """Simple SQLite-backed history store for build outcomes."""

    def __init__(self, path: Path):
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        self._ensure_schema()

    def _ensure_schema(self) -> None:
        with sqlite3.connect(self.path) as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS build_events (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    run_id TEXT,
                    timestamp TEXT,
                    name TEXT,
                    version TEXT,
                    python_tag TEXT,
                    platform_tag TEXT,
                    status TEXT,
                    source_spec TEXT,
                    detail TEXT,
                    wheel_path TEXT,
                    cached INTEGER,
                    metadata_json TEXT
                )
                """
            )
            conn.execute("CREATE INDEX IF NOT EXISTS idx_build_events_name ON build_events(name)")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_build_events_name_version ON build_events(name, version)")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_build_events_status ON build_events(status)")
            conn.commit()

    def record_event(
        self,
        *,
        run_id: str,
        name: str,
        version: str,
        python_tag: str,
        platform_tag: str,
        status: str,
        source_spec: Optional[str] = None,
        detail: Optional[str] = None,
        wheel_path: Optional[str] = None,
        cached: bool = False,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> None:
        payload = {
            "run_id": run_id,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "name": name,
            "version": version,
            "python_tag": python_tag,
            "platform_tag": platform_tag,
            "status": status,
            "source_spec": source_spec,
            "detail": detail,
            "wheel_path": wheel_path,
            "cached": 1 if cached else 0,
            "metadata_json": json.dumps(metadata or {}),
        }
        with self._lock, sqlite3.connect(self.path) as conn:
            conn.execute(
                """
                INSERT INTO build_events (
                    run_id, timestamp, name, version, python_tag, platform_tag,
                    status, source_spec, detail, wheel_path, cached, metadata_json
                )
                VALUES (
                    :run_id, :timestamp, :name, :version, :python_tag, :platform_tag,
                    :status, :source_spec, :detail, :wheel_path, :cached, :metadata_json
                )
                """,
                payload,
            )
            conn.commit()

    def recent(self, *, limit: int = 20, status: Optional[str] = None) -> List["BuildEvent"]:
        query = "SELECT * FROM build_events"
        params: List[Any] = []
        if status:
            query += " WHERE status = ?"
            params.append(status)
        query += " ORDER BY id DESC LIMIT ?"
        params.append(limit)
        with sqlite3.connect(self.path) as conn:
            rows = conn.execute(query, params).fetchall()
        return [_row_to_event(row) for row in rows]

    def top_failures(self, *, limit: int = 20, statuses: Iterable[str] = ("failed", "missing")) -> List["FailureStat"]:
        placeholders = ",".join("?" for _ in statuses)
        query = f"""
            SELECT name, COUNT(*) as failures
            FROM build_events
            WHERE status IN ({placeholders})
            GROUP BY name
            ORDER BY failures DESC
            LIMIT ?
        """
        params: List[Any] = list(statuses) + [limit]
        with sqlite3.connect(self.path) as conn:
            rows = conn.execute(query, params).fetchall()
        return [FailureStat(name=row[0], failures=row[1]) for row in rows]

    def top_slowest(self, *, limit: int = 10) -> List["DurationStat"]:
        query = """
            SELECT name,
                   AVG(CAST(json_extract(metadata_json, '$.duration_seconds') AS REAL)) as avg_duration,
                   SUM(CASE WHEN status IN ('failed', 'missing', 'failed_attempt', 'system_recipe_failed') THEN 1 ELSE 0 END) as failures
            FROM build_events
            WHERE metadata_json LIKE '%duration_seconds%'
            GROUP BY name
            ORDER BY avg_duration DESC
            LIMIT ?
        """
        with sqlite3.connect(self.path) as conn:
            rows = conn.execute(query, (limit,)).fetchall()
        return [DurationStat(name=row[0], avg_duration=row[1], failures=row[2]) for row in rows]

    def variant_success_rate(self, name: str) -> dict:
        query = """
            SELECT json_extract(metadata_json, '$.variant') as variant,
                   SUM(CASE WHEN status = 'built' THEN 1 ELSE 0 END) as success,
                   COUNT(*) as total
            FROM build_events
            WHERE name = ? AND metadata_json LIKE '%variant%'
            GROUP BY variant
        """
        rates = {}
        with sqlite3.connect(self.path) as conn:
            rows = conn.execute(query, (name,)).fetchall()
        for row in rows:
            variant = row[0] or "unknown"
            success = row[1]
            total = row[2] or 1
            rates[variant] = success / total
        return rates

    def package_summary(self, name: str) -> "PackageSummary":
        query = """
            SELECT status, COUNT(*) FROM build_events
            WHERE name = ?
            GROUP BY status
        """
        with sqlite3.connect(self.path) as conn:
            rows = conn.execute(query, (name,)).fetchall()
            latest_row = conn.execute(
                "SELECT * FROM build_events WHERE name = ? ORDER BY id DESC LIMIT 1", (name,)
            ).fetchone()
            durations = conn.execute(
                "SELECT AVG(CAST(json_extract(metadata_json, '$.duration_seconds') AS REAL)) FROM build_events WHERE name = ? AND metadata_json LIKE '%duration_seconds%'",
                (name,),
            ).fetchone()
        status_counts = {row[0]: row[1] for row in rows}
        latest = _row_to_event(latest_row) if latest_row else None
        avg_duration = durations[0] if durations and durations[0] is not None else None
        return PackageSummary(name=name, status_counts=status_counts, latest=latest, avg_duration=avg_duration)

    def export_csv(self, path: Path, *, limit: int = 0) -> None:
        query = "SELECT * FROM build_events ORDER BY id DESC"
        if limit > 0:
            query += f" LIMIT {int(limit)}"
        with sqlite3.connect(self.path) as conn, path.open("w", encoding="utf-8") as fh:
            cursor = conn.execute(query)
            headers = [col[0] for col in cursor.description]
            fh.write(",".join(headers) + "\n")
            for row in cursor.fetchall():
                line = ",".join(_csv_escape(str(item)) for item in row)
                fh.write(line + "\n")

    def last_event(self, name: str, version: Optional[str] = None) -> Optional["BuildEvent"]:
        query = "SELECT * FROM build_events WHERE name = ?"
        params: List[Any] = [name]
        if version:
            query += " AND version = ?"
            params.append(version)
        query += " ORDER BY id DESC LIMIT 1"
        with sqlite3.connect(self.path) as conn:
            row = conn.execute(query, params).fetchone()
        return _row_to_event(row) if row else None


@dataclass
class BuildEvent:
    run_id: str
    timestamp: str
    name: str
    version: str
    python_tag: str
    platform_tag: str
    status: str
    source_spec: Optional[str]
    detail: Optional[str]
    wheel_path: Optional[str]
    cached: bool
    metadata: Dict[str, Any]


@dataclass
class FailureStat:
    name: str
    failures: int


@dataclass
class PackageSummary:
    name: str
    status_counts: Dict[str, int]
    latest: Optional[BuildEvent]
    avg_duration: Optional[float] = None


@dataclass
class DurationStat:
    name: str
    avg_duration: float
    failures: int = 0


def _row_to_event(row: tuple) -> BuildEvent:
    (
        _id,
        run_id,
        timestamp,
        name,
        version,
        python_tag,
        platform_tag,
        status,
        source_spec,
        detail,
        wheel_path,
        cached,
        metadata_json,
    ) = row
    metadata = json.loads(metadata_json or "{}")
    return BuildEvent(
        run_id=run_id,
        timestamp=timestamp,
        name=name,
        version=version,
        python_tag=python_tag,
        platform_tag=platform_tag,
        status=status,
        source_spec=source_spec,
        detail=detail,
        wheel_path=wheel_path,
        cached=bool(cached),
        metadata=metadata,
    )


def _csv_escape(value: str) -> str:
    if any(ch in value for ch in {",", '"', "\n"}):
        return '"' + value.replace('"', '""') + '"'
    return value
