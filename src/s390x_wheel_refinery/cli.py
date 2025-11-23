from __future__ import annotations

import argparse
import json
import logging
import shutil
import sys
from dataclasses import asdict
from pathlib import Path
from typing import List
from uuid import uuid4
from concurrent.futures import ThreadPoolExecutor, as_completed

import uvicorn

from . import builder as builder_module
from .config import build_config
from .history import BuildHistory
from .index import IndexClient
from .manifest import write_manifest
from .models import Manifest, ManifestEntry
from .resolver import build_plan
from .web import create_app
from .scanner import scan_wheels

LOG = logging.getLogger("s390x_wheel_refinery")


def parse_args(argv: List[str] | None = None) -> argparse.Namespace:
    argv = list(argv) if argv is not None else sys.argv[1:]
    if argv and argv[0] == "history":
        return _parse_history_args(argv[1:])
    if argv and argv[0] == "serve":
        return _parse_serve_args(argv[1:])
    return _parse_run_args(argv)


def _parse_run_args(argv: List[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Rebuild wheels for s390x.")
    parser.add_argument("--input", required=True, type=Path, help="Directory containing foreign wheels.")
    parser.add_argument("--output", required=True, type=Path, help="Directory to write s390x wheels to.")
    parser.add_argument("--cache", required=True, type=Path, help="Shared cache directory.")
    parser.add_argument("--python", required=True, dest="python_version", help="Target Python version (e.g. 3.11).")
    parser.add_argument("--platform-tag", default="manylinux2014_s390x", help="Target platform tag (default: manylinux2014_s390x).")
    parser.add_argument("--upgrade-strategy", default="pinned", help="Upgrade strategy (pinned or eager).")
    parser.add_argument("--config", type=Path, help="Optional JSON/TOML config file.")
    parser.add_argument("--index-url", help="Primary package index URL.")
    parser.add_argument("--extra-index-url", action="append", default=[], help="Additional package indexes.")
    parser.add_argument("--trusted-host", action="append", default=[], help="Trusted hosts for pip operations.")
    parser.add_argument("--history-db", type=Path, help="Path to history database (defaults to <cache>/history.db).")
    parser.add_argument("--allow-system-recipes", action="store_true", help="Allow executing system recipes from overrides.")
    parser.add_argument("--no-system-recipes", action="store_true", help="Disable executing system recipes.")
    parser.add_argument("--dry-run-recipes", action="store_true", help="Log system recipes without executing them.")
    parser.add_argument("--max-attempts", type=int, default=None, help="Max build attempts per package (default 3).")
    parser.add_argument("--attempt-timeout", type=int, default=None, help="Timeout per build attempt in seconds (default 900).")
    parser.add_argument("--attempt-backoff-base", type=int, default=None, help="Base backoff between attempts in seconds (default 5).")
    parser.add_argument("--attempt-backoff-max", type=int, default=None, help="Max backoff between attempts in seconds (default 60).")
    parser.add_argument("--container-image", help="Container image to run builds in (bind-mounts cache/output).")
    parser.add_argument("--container-engine", default=None, help="Container engine (docker/podman). Defaults to docker.")
    parser.add_argument(
        "--container-preset",
        choices=["rocky", "fedora", "ubuntu"],
        help="Preset container base to use when container-image is not provided.",
    )
    parser.add_argument("--container-cpu", default=None, help="Container CPU limit (passed to engine, e.g., 2 or 0.5).")
    parser.add_argument("--container-memory", default=None, help="Container memory limit (passed to engine, e.g., 4g).")
    parser.add_argument("--auto-apply-suggestions", action="store_true", help="Automatically add suggested system packages/recipes from hints.")
    parser.add_argument("--jobs", type=int, default=1, help="Number of concurrent build jobs (default 1).")
    parser.add_argument("--fallback-latest", action="store_true", help="If pinned builds fail, retry once with latest compatible version.")
    parser.add_argument("--schedule", default="shortest-first", help="Scheduling strategy: shortest-first or fifo.")
    parser.add_argument(
        "--manifest",
        type=Path,
        help="Manifest output path (defaults to <output>/manifest.json).",
    )
    parser.add_argument("--skip-known-failures", action="store_true", help="Skip builds whose last history entry failed/missing.")
    parser.add_argument("--verbose", action="store_true", help="Enable debug logging.")
    ns = parser.parse_args(argv)
    ns.command = "run"
    return ns


def _parse_history_args(argv: List[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Inspect build history.")
    parser.add_argument("--history-db", "--db", dest="history_db", type=Path, default=Path("history.db"), help="Path to history database.")
    parser.add_argument("--recent", type=int, default=20, help="Number of recent events to show.")
    parser.add_argument("--status", help="Filter recent events by status (e.g. built, failed, missing, reused).")
    parser.add_argument("--top-failures", type=int, default=5, help="Show top N failing packages.")
    parser.add_argument("--package", help="Show summary for a specific package.")
    parser.add_argument("--export-csv", type=Path, help="Export events to CSV at the given path.")
    parser.add_argument("--export-limit", type=int, default=0, help="Limit rows when exporting CSV (0 = all).")
    parser.add_argument("--json", action="store_true", help="Output JSON instead of text.")
    return parser.parse_args(argv, namespace=argparse.Namespace(command="history"))


def _parse_serve_args(argv: List[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Serve history web UI.")
    parser.add_argument("--history-db", "--db", dest="history_db", type=Path, default=Path("history.db"), help="Path to history database.")
    parser.add_argument("--host", default="0.0.0.0", help="Host to bind.")
    parser.add_argument("--port", type=int, default=8000, help="Port to bind.")
    parser.add_argument("--reload", action="store_true", help="Enable autoreload (dev).")
    return parser.parse_args(argv, namespace=argparse.Namespace(command="serve"))


def main(argv: List[str] | None = None) -> int:
    args = parse_args(argv)
    if getattr(args, "command", "run") == "history":
        return _run_history(args)
    if getattr(args, "command", "run") == "serve":
        return _run_server(args)

    logging.basicConfig(level=logging.DEBUG if args.verbose else logging.INFO, format="%(levelname)s %(message)s")

    config = build_config(
        target_python=args.python_version,
        target_platform_tag=args.platform_tag,
        upgrade_strategy=args.upgrade_strategy,
        index_url=args.index_url,
        extra_index_urls=args.extra_index_url,
        trusted_hosts=args.trusted_host,
        config_file=args.config,
        allow_system_recipes=False if args.no_system_recipes else (True if args.allow_system_recipes else None),
        dry_run_recipes=args.dry_run_recipes if args.dry_run_recipes else None,
        max_attempts=args.max_attempts,
        attempt_timeout=args.attempt_timeout,
        attempt_backoff_base=args.attempt_backoff_base,
        attempt_backoff_max=args.attempt_backoff_max,
        container_image=args.container_image,
        container_engine=args.container_engine,
        container_preset=args.container_preset,
        auto_apply_suggestions=args.auto_apply_suggestions,
        fallback_latest=args.fallback_latest,
        container_cpu=args.container_cpu,
        container_memory=args.container_memory,
    )

    run_id = uuid4().hex
    history_path = args.history_db or args.cache / "history.db"
    history = BuildHistory(history_path)
    index_client = IndexClient(config.index)

    LOG.info("Scanning input directory %s", args.input)
    wheels = scan_wheels(args.input)
    plan = build_plan(wheels, config, index_client=index_client)
    builder = builder_module.WheelBuilder(args.cache, args.output, config, history=history, run_id=run_id, index_client=index_client)

    manifest_entries: List[ManifestEntry] = []

    for reusable in plan.reusable:
        src = Path(reusable.path)
        dest = args.output / src.name
        args.output.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dest)
        manifest_entries.append(
            ManifestEntry(
                name=reusable.name,
                version=reusable.version,
                status="reused",
                path=str(dest),
                detail="Pure Python or already compatible",
            )
        )
        history.record_event(
            run_id=run_id,
            name=reusable.name,
            version=reusable.version,
            python_tag=config.python_tag,
            platform_tag=config.target_platform_tag,
            status="reused",
            source_spec="input wheel",
            detail="Pure Python or already compatible",
            wheel_path=str(dest),
            cached=True,
            metadata={"source": "input"},
        )

    jobs_to_run = []
    for job in plan.to_build:
        if args.skip_known_failures:
            last = history.last_event(job.name, job.version)
            if last and last.status in {"failed", "missing", "system_recipe_failed"}:
                detail = f"Skipped: last status {last.status} at {last.timestamp}"
                manifest_entries.append(
                    ManifestEntry(
                        name=job.name,
                        version=job.version,
                        status="skipped_known_failure",
                        detail=detail,
                    )
                )
                history.record_event(
                    run_id=run_id,
                    name=job.name,
                    version=job.version,
                    python_tag=config.python_tag,
                    platform_tag=config.target_platform_tag,
                    status="skipped_known_failure",
                    source_spec=job.source_spec,
                    detail=detail,
                )
                LOG.info("Skipping %s==%s due to known failure in history", job.name, job.version)
                continue
        jobs_to_run.append(job)

    from .scheduler import schedule_jobs
    jobs_to_run = schedule_jobs(jobs_to_run, history, strategy=args.schedule)

    builder.ensure_ready()
    if args.jobs <= 1:
        for job in jobs_to_run:
            _run_single_build(builder, job, manifest_entries, history, config, run_id)
    else:
        with ThreadPoolExecutor(max_workers=args.jobs) as executor:
            futures = {executor.submit(builder.build_job, job): job for job in jobs_to_run}
            for future in as_completed(futures):
                job = futures[future]
                try:
                    result = future.result()
                    manifest_entries.append(result.entry)
                    _enqueue_parents(jobs_to_run, job, history, builder, manifest_entries, config, run_id, executor)
                except Exception as exc:  # noqa: BLE001
                    meta = None
                    if isinstance(exc, builder_module.BuildAttemptError):
                        meta = {
                            "log_path": str(exc.log_path),
                            "hint": exc.hint,
                            "duration_seconds": exc.duration,
                        }
                    LOG.error("Build failed for %s: %s", job.name, exc)
                    manifest_entries.append(
                        ManifestEntry(
                            name=job.name,
                            version=job.version,
                            status="failed",
                            detail=str(exc),
                            metadata=meta,
                        )
                    )
                    history.record_event(
                        run_id=run_id,
                        name=job.name,
                        version=job.version,
                        python_tag=config.python_tag,
                        platform_tag=config.target_platform_tag,
                        status="failed",
                        source_spec=job.source_spec,
                        detail=str(exc),
                        metadata=meta,
                    )

    for missing in plan.missing_requirements:
        manifest_entries.append(
            ManifestEntry(
                name=missing,
                version="unknown",
                status="missing",
                detail="No pinned version to build; please provide override or input wheel.",
            )
        )
        history.record_event(
            run_id=run_id,
            name=missing,
            version="unknown",
            python_tag=config.python_tag,
            platform_tag=config.target_platform_tag,
            status="missing",
            source_spec=missing,
            detail="No pinned version to build; provide override or wheel.",
        )

    if plan.dependency_expansions:
        for job in plan.dependency_expansions:
            history.record_event(
                run_id=run_id,
                name=job.name,
                version=job.version,
                python_tag=config.python_tag,
                platform_tag=config.target_platform_tag,
                status="planned_dependency_expansion",
                source_spec=job.source_spec,
                detail="Auto-planned dependency expansion",
            )

    manifest_path = args.manifest or args.output / "manifest.json"
    manifest = Manifest(
        python_tag=config.python_tag,
        platform_tag=config.target_platform_tag,
        entries=manifest_entries,
    )
    write_manifest(manifest, manifest_path)
    LOG.info("Manifest written to %s", manifest_path)
    exit_code = 0
    if any(entry.status in {"failed", "missing"} for entry in manifest_entries):
        exit_code = 1
    return exit_code


def _run_single_build(builder, job, manifest_entries, history, config, run_id):
    try:
        result = builder.build_job(job)
        manifest_entries.append(result.entry)
        _enqueue_parents([], job, history, builder, manifest_entries, config, run_id, None)
    except Exception as exc:  # noqa: BLE001
        meta = None
        if isinstance(exc, builder_module.BuildAttemptError):
            meta = {
                "log_path": str(exc.log_path),
                "hint": exc.hint,
                "duration_seconds": exc.duration,
            }
        LOG.error("Build failed for %s: %s", job.name, exc)
        manifest_entries.append(
            ManifestEntry(
                name=job.name,
                version=job.version,
                status="failed",
                detail=str(exc),
                metadata=meta,
            )
        )
        history.record_event(
            run_id=run_id,
            name=job.name,
            version=job.version,
            python_tag=config.python_tag,
            platform_tag=config.target_platform_tag,
            status="failed",
            source_spec=job.source_spec,
            detail=str(exc),
            metadata=meta,
        )


def _enqueue_parents(queue, job, history, builder, manifest_entries, config, run_id, executor):
    if not job.parents:
        return
    parent_names = {p.lower() for p in job.parents}
    for parent_name in parent_names:
        # Avoid duplicates; only enqueue if not completed
        key = (parent_name, job.version)
        if key in getattr(builder, "_completed", set()):
            continue
        parent_job = builder_module.BuildJob(
            name=parent_name,
            version="latest",
            python_tag=config.python_tag,
            platform_tag=config.target_platform_tag,
            source_spec=parent_name,
            reason="requeued after dependency build",
            depth=job.depth + 1,
            parents=[],
            children=[],
        )
        history.record_event(
            run_id=run_id,
            name=parent_job.name,
            version=parent_job.version,
            python_tag=config.python_tag,
            platform_tag=config.target_platform_tag,
            status="requeued_parent",
            source_spec=parent_job.source_spec,
            detail="Requeued after dependency build",
        )
        if executor:
            fut = executor.submit(builder.build_job, parent_job)
            try:
                result = fut.result()
                manifest_entries.append(result.entry)
            except Exception as exc:  # noqa: BLE001
                meta = None
                if isinstance(exc, builder_module.BuildAttemptError):
                    meta = {
                        "log_path": str(exc.log_path),
                        "hint": exc.hint,
                        "duration_seconds": exc.duration,
                    }
                manifest_entries.append(
                    builder_module.ManifestEntry(
                        name=parent_job.name,
                        version=parent_job.version,
                        status="failed",
                        detail=str(exc),
                        metadata=meta,
                    )
                )
        else:
            result = builder.build_job(parent_job)
            manifest_entries.append(result.entry)


def _run_history(args: argparse.Namespace) -> int:
    history = BuildHistory(args.history_db)
    recent = history.recent(limit=args.recent, status=args.status)
    failures = history.top_failures(limit=args.top_failures) if args.top_failures else []
    summary = history.package_summary(args.package) if args.package else None

    if args.export_csv:
        history.export_csv(args.export_csv, limit=args.export_limit)

    if args.json:
        payload = {
            "recent": [asdict(event) for event in recent],
            "top_failures": [asdict(stat) for stat in failures],
            "summary": asdict(summary) if summary else None,
        }
        print(json.dumps(payload, indent=2))
        return 0

    print(f"History DB: {args.history_db}")
    print(f"Recent events (limit {args.recent}{' filtered by ' + args.status if args.status else ''}):")
    for event in recent:
        cached_flag = "cached" if event.cached else "built"
        print(
            f"- [{event.timestamp}] {event.status.upper():7} {event.name} {event.version} "
            f"{event.python_tag}/{event.platform_tag} ({cached_flag})"
        )
        if event.detail:
            print(f"    detail: {event.detail}")
        if event.wheel_path:
            print(f"    wheel: {event.wheel_path}")

    if failures:
        print(f"\nTop {len(failures)} failing packages:")
        for stat in failures:
            print(f"- {stat.name}: {stat.failures} failures")
    if summary:
        print(f"\nPackage summary for {summary.name}:")
        for status, count in summary.status_counts.items():
            print(f"- {status}: {count}")
        if summary.latest:
            print(f"Latest: {summary.latest.status} {summary.latest.version} at {summary.latest.timestamp}")
    if args.export_csv:
        print(f"\nExported CSV to {args.export_csv}")
    return 0


def _run_server(args: argparse.Namespace) -> int:
    history = BuildHistory(args.history_db)
    app = create_app(history)
    uvicorn.run(app, host=args.host, port=args.port, reload=args.reload)
    return 0


if __name__ == "__main__":
    sys.exit(main())
