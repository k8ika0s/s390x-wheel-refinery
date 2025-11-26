from __future__ import annotations

import logging
import os
import shutil
import subprocess
import sys
import tarfile
import re
import shlex
import copy
import time
from dataclasses import dataclass
from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Iterable, Optional
from zipfile import ZipFile

from packaging.utils import parse_wheel_filename

from .config import PackageOverride, RefineryConfig
from .history import BuildHistory
from .hints import HintCatalog
from .models import BuildJob, ManifestEntry
from .index import IndexClient

LOG = logging.getLogger(__name__)


@dataclass
class BuildResult:
    entry: ManifestEntry
    cached: bool = False


@dataclass
class BuildVariant:
    name: str
    build_isolation: bool = True
    env_patch: dict = None
    extra_build_args: list[str] = None


class BuildAttemptError(RuntimeError):
    def __init__(
        self,
        message: str,
        log_path: Path,
        variant: str,
        attempt: int,
        hint: Optional[str] = None,
        duration: Optional[float] = None,
    ):
        super().__init__(message)
        self.log_path = log_path
        self.variant = variant
        self.attempt = attempt
        self.hint = hint
        self.duration = duration


class WheelBuilder:
    def __init__(
        self,
        cache_dir: Path,
        output_dir: Path,
        config: RefineryConfig,
        *,
        history: Optional[BuildHistory] = None,
        run_id: str = "local",
        index_client: Optional[IndexClient] = None,
    ):
        self.cache_dir = cache_dir
        self.output_dir = output_dir
        self.config = config
        self.venv_dir = cache_dir / "venv"
        self.cache_wheel_dir = cache_dir / "wheels"
        self._venv_python: Optional[Path] = None
        self.history = history
        self.run_id = run_id
        self._container_image = config.container_image or _default_image_for_preset(config.container_preset)
        self._container_engine = config.container_engine
        self._ensure_ready_once = False
        self.index_client = index_client
        self.hint_catalog = HintCatalog(Path(__file__).parent.parent / "data" / "hints.yaml")
        self._completed: set[tuple[str, str]] = set()
        self._attempt_counts: dict[tuple[str, str], int] = {}

    def ensure_ready(self) -> None:
        if self._ensure_ready_once:
            return
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self.cache_wheel_dir.mkdir(parents=True, exist_ok=True)
        if not self.venv_dir.exists():
            LOG.info("Creating venv at %s", self.venv_dir)
            self._run([sys.executable, "-m", "venv", str(self.venv_dir)])
        self._venv_python = self.venv_dir / "bin" / "python"
        self._run(
            [
                str(self._venv_python),
                "-m",
                "pip",
                "install",
                *self._pip_index_args(),
                "--upgrade",
                "pip",
                "build",
            ],
            env=self._pip_env(),
        )
        self._ensure_ready_once = True

    def build_job(self, job: BuildJob) -> BuildResult:
        self.ensure_ready()
        key = (job.name.lower(), job.version)
        attempts = self._attempt_counts.get(key, 0)
        if attempts >= self.config.max_attempts:
            raise RuntimeError(f"Exceeded max attempts for {job.name}=={job.version}")
        override = self._override_for(job)
        recipe_ran = False
        if override and override.system_recipe:
            recipe_ran = self._run_system_recipe(job, override)
        cached = self._find_cached(job)
        if cached:
            target = self._copy_to_output(cached)
            entry = ManifestEntry(
                name=job.name,
                version=job.version,
                status="cached",
                path=str(target),
                detail=self._detail_with_overrides("Reused cached wheel", override),
            )
            self._record_history(entry, job, override, cached=True, recipe_ran=recipe_ran)
            return BuildResult(entry=entry, cached=True)

        variants = self._variants(job.name)
        attempts = max(1, self.config.max_attempts)
        last_error: Optional[Exception] = None
        last_hint_steps: list[str] = []
        for attempt in range(1, attempts + 1):
            variant = variants[min(attempt - 1, len(variants) - 1)]
            try:
                entry = self._attempt_build(job, override, variant, attempt, recipe_ran)
                return BuildResult(entry=entry, cached=False)
            except Exception as exc:  # noqa: BLE001
                last_error = exc
                detail = str(exc)
                log_path = None
                variant_name = variant.name
                hint = None
                duration = None
                if isinstance(exc, BuildAttemptError):
                    log_path = exc.log_path
                    variant_name = exc.variant
                    hint = exc.hint
                    duration = exc.duration
                    if hint and self.config.auto_apply_suggestions:
                        self._apply_suggestion(job, hint)
                    if hint:
                        last_hint_steps = self._recipe_steps_from_hint(hint)
                if self.history:
                    self.history.record_event(
                        run_id=self.run_id,
                        name=job.name,
                        version=job.version,
                        python_tag=job.python_tag,
                        platform_tag=job.platform_tag,
                        status="failed_attempt",
                        source_spec=job.source_spec,
                        detail=detail,
                        metadata={
                            "variant": variant_name,
                            "attempt": attempt,
                            "log_path": str(log_path) if log_path else None,
                            "hint": hint,
                            "duration": duration,
                        },
                    )
                LOG.warning(
                    "Attempt %s/%s failed for %s: %s (variant=%s log=%s)",
                    attempt,
                    attempts,
                    job.source_spec,
                    exc,
                    variant_name,
                    log_path,
                )
                # Backoff before next attempt
                if attempt < attempts:
                    delay = min(
                        self.config.attempt_backoff_max,
                        self.config.attempt_backoff_base * (2 ** (attempt - 1)),
                    )
                    LOG.info("Backing off for %ss before next attempt for %s", delay, job.source_spec)
                    time.sleep(delay)
        # Optional single retry with hint-derived recipe
        if last_error and last_hint_steps:
            LOG.info("Retrying %s with hint-derived recipe steps: %s", job.source_spec, "; ".join(last_hint_steps))
            temp_override = copy.deepcopy(override) if override else PackageOverride()
            for step in last_hint_steps:
                if step not in temp_override.system_recipe:
                    temp_override.system_recipe.append(step)
            try:
                entry = self._attempt_build(
                    job,
                    temp_override,
                    variants[0],
                    attempts + 1,
                    recipe_ran=True,
                )
                return BuildResult(entry=entry, cached=False)
            except Exception as exc:  # noqa: BLE001
                last_error = exc

        raise RuntimeError(f"Exhausted {attempts} attempts for {job.source_spec}: {last_error}")
        # Fallback latest retry (optional) happens in caller (CLI/resolver) if enabled.

    def _find_cached(self, job: BuildJob) -> Optional[Path]:
        for wheel_path in sorted(self.cache_wheel_dir.glob("*.whl")):
            try:
                name, version, _, tags = parse_wheel_filename(wheel_path.name)
            except Exception:
                continue
            if name.lower() != job.name.lower() or str(version) != job.version:
                continue
            for tag in tags:
                interpreter = getattr(tag, "interpreter", "")
                platform = getattr(tag, "platform", "")
                python_ok = interpreter in {job.python_tag, "py3"} or interpreter.startswith("py3")
                platform_ok = platform == "any" or platform == job.platform_tag or (
                    platform.endswith("_s390x") and job.platform_tag.endswith("_s390x")
                )
                if python_ok and platform_ok:
                    return wheel_path
        return None

    def _copy_to_output(self, wheel_path: Path) -> Path:
        self.output_dir.mkdir(parents=True, exist_ok=True)
        target = self.output_dir / wheel_path.name
        shutil.copy2(wheel_path, target)
        return target

    def _attempt_build(
        self,
        job: BuildJob,
        override: Optional[PackageOverride],
        variant: BuildVariant,
        attempt: int,
        recipe_ran: bool,
    ) -> ManifestEntry:
        build_env = self._env_for(job, override=override, variant=variant)
        log_dir = self.cache_dir / "logs"
        log_dir.mkdir(parents=True, exist_ok=True)
        log_path = log_dir / f"{job.name}-{job.version}-attempt{attempt}-{variant.name}.log"
        start_time = time.time()

        with TemporaryDirectory(prefix=f"build-{job.name}-{job.version}-", dir=self.cache_dir) as work_dir:
            work_path = Path(work_dir)
            download_dir = work_path / "downloads"
            download_dir.mkdir(parents=True, exist_ok=True)

            LOG.info("Downloading source for %s (variant=%s attempt=%s)", job.source_spec, variant.name, attempt)
            self._run_capture(
                [
                    str(self._venv_python),
                    "-m",
                    "pip",
                    "download",
                    "--no-deps",
                    "--no-binary",
                    ":all:",
                    "-d",
                    str(download_dir),
                    job.source_spec,
                    *self._pip_index_args(),
                ],
                env=build_env,
                log_path=log_path,
                step="download",
                variant_name=variant.name,
                attempt=attempt,
                job=job,
            )

            sdist = _first_sdist(download_dir.iterdir())
            if not sdist:
                raise RuntimeError(f"No sdist found for {job.source_spec} in {download_dir}")

            source_dir = work_path / "src"
            source_dir.mkdir(parents=True, exist_ok=True)
            _extract_sdist(sdist, source_dir)

            LOG.info("Building wheel for %s (variant=%s attempt=%s)", job.source_spec, variant.name, attempt)
            build_cmd = [
                str(self._venv_python),
                "-m",
                "build",
                "--wheel",
                "--outdir",
                str(self.cache_wheel_dir),
            ]
            if not variant.build_isolation:
                build_cmd.append("--no-isolation")
            build_cmd.extend(variant.extra_build_args or [])
            build_cmd.append(str(source_dir))
            self._run_capture(
                build_cmd,
                env=build_env,
                log_path=log_path,
                step="build",
                variant_name=variant.name,
                attempt=attempt,
                job=job,
            )

        built = self._find_cached(job)
        if not built:
            raise RuntimeError(f"Build finished but wheel not found for {job.name}=={job.version}")
        target = self._copy_to_output(built)
        self._attempt_counts[(job.name.lower(), job.version)] = attempt
        duration = time.time() - start_time
        detail = self._detail_with_overrides(
            f"{job.reason}; variant={variant.name}; attempt={attempt}; recipe_ran={recipe_ran}; log={log_path}",
            override,
        )
        entry = ManifestEntry(
            name=job.name,
            version=job.version,
            status="built",
            path=str(target),
            detail=detail,
            metadata={"variant": variant.name, "attempt": attempt, "log_path": str(log_path), "duration_seconds": duration},
        )
        self._record_history(
            entry,
            job,
            override,
            cached=False,
            recipe_ran=recipe_ran,
            metadata={
                "variant": variant.name,
                "attempt": attempt,
                "log_path": str(log_path),
                "duration_seconds": duration,
                "parents": job.parents,
                "children": job.children,
            },
        )
        self._completed.add((job.name.lower(), job.version))
        return entry

    def _env_for(self, job: BuildJob, *, override: Optional[PackageOverride] = None, variant: Optional[BuildVariant] = None) -> dict:
        env = self._pip_env()
        if override:
            env.update(override.env)
        if variant and variant.env_patch:
            env.update(variant.env_patch)
        return env

    def _run(self, cmd: Iterable[str], *, env: Optional[dict] = None) -> None:
        LOG.debug("Running: %s", " ".join(cmd))
        subprocess.run(cmd, check=True, env=env)

    def _run_capture(
        self,
        cmd: Iterable[str],
        *,
        env: Optional[dict],
        log_path: Path,
        step: str,
        variant_name: str,
        attempt: int,
        job: Optional[BuildJob] = None,
    ) -> None:
        log_path.parent.mkdir(parents=True, exist_ok=True)
        LOG.debug("Running (%s): %s", step, " ".join(cmd))
        full_cmd = list(cmd)
        use_container = bool(self._container_image)
        if use_container:
            cpu = job.resource_cpu if job and job.resource_cpu else None
            mem = job.resource_mem if job and job.resource_mem else None
            full_cmd = self._containerized_command(full_cmd, env, workdir=None, cpu=cpu, mem=mem)

        try:
            proc = subprocess.run(
                full_cmd,
                env=None if use_container else env,
                capture_output=True,
                text=True,
                timeout=self.config.attempt_timeout,
            )
        except subprocess.TimeoutExpired as exc:
            with log_path.open("a", encoding="utf-8") as fh:
                fh.write(f"== step: {step} (timeout)\n")
                fh.write(exc.stdout or "")
                fh.write(exc.stderr or "")
            raise BuildAttemptError(
                f"{step} timed out after {self.config.attempt_timeout}s. Log: {log_path}",
                log_path,
                variant=variant_name,
                attempt=attempt,
                hint="Increase attempt timeout or check hanging build step",
                duration=self.config.attempt_timeout,
            )
        with log_path.open("a", encoding="utf-8") as fh:
            fh.write(f"== step: {step}\n")
            fh.write(proc.stdout or "")
            fh.write(proc.stderr or "")
        if proc.returncode != 0:
            hint = self._hint_from_logs(proc.stderr or proc.stdout or "")
            message = f"{step} failed (rc={proc.returncode}). Log: {log_path}"
            if hint:
                message += f" Hint: {hint}"
            raise BuildAttemptError(message, log_path, variant=variant_name, attempt=attempt, hint=hint, duration=self.config.attempt_timeout)

    def _run_system_recipe(self, job: BuildJob, override: PackageOverride) -> None:
        if not self.config.allow_system_recipes:
            LOG.info("Skipping system recipe for %s (disabled)", job.name)
            return False
        env = self._env_for(job, override=override)
        if self.config.dry_run_recipes:
            for step in override.system_recipe:
                LOG.info("Dry-run recipe for %s: %s", job.name, step)
            return False
        success = True
        for step in override.system_recipe:
            LOG.info("Running system recipe for %s: %s", job.name, step)
            try:
                proc_cmd = ["/bin/sh", "-c", step]
                if self._container_image:
                    proc_cmd = self._containerized_command(proc_cmd, env, workdir=None)
                proc = subprocess.run(proc_cmd, env=None if self._container_image else env, capture_output=True, text=True)
                if proc.returncode != 0:
                    raise subprocess.CalledProcessError(proc.returncode, step, proc.stdout, proc.stderr)
                if self.history:
                    self.history.record_event(
                        run_id=self.run_id,
                        name=job.name,
                        version=job.version,
                        python_tag=job.python_tag,
                        platform_tag=job.platform_tag,
                        status="system_recipe_ran",
                        source_spec=job.source_spec,
                        detail=f"Step succeeded: {step}",
                        metadata={"stdout": proc.stdout[-5000:], "stderr": proc.stderr[-5000:], "step": step},
                    )
            except subprocess.CalledProcessError as exc:
                success = False
                LOG.error("System recipe step failed for %s: %s", job.name, exc)
                if self.history:
                    self.history.record_event(
                        run_id=self.run_id,
                        name=job.name,
                        version=job.version,
                        python_tag=job.python_tag,
                        platform_tag=job.platform_tag,
                        status="system_recipe_failed",
                        source_spec=job.source_spec,
                        detail=str(exc),
                        metadata={"step": step, "stdout": (exc.stdout or "")[-5000:], "stderr": (exc.stderr or "")[-5000:]},
                    )
                raise
        return success

    def _pip_index_args(self) -> list[str]:
        args: list[str] = []
        if self.config.index.index_url:
            args.extend(["--index-url", self.config.index.index_url])
        for extra in self.config.index.extra_index_urls:
            args.extend(["--extra-index-url", extra])
        for host in self.config.index.trusted_hosts:
            args.extend(["--trusted-host", host])
        return args

    def _pip_env(self) -> dict:
        env = os.environ.copy()
        if self.config.index.index_url:
            env.setdefault("PIP_INDEX_URL", self.config.index.index_url)
        if self.config.index.extra_index_urls:
            env.setdefault("PIP_EXTRA_INDEX_URL", " ".join(self.config.index.extra_index_urls))
        if self.config.index.trusted_hosts:
            env.setdefault("PIP_TRUSTED_HOST", " ".join(self.config.index.trusted_hosts))
        return env

    def _override_for(self, job: BuildJob) -> Optional[PackageOverride]:
        return self.config.overrides.get(job.name) or self.config.overrides.get(job.name.lower())

    def _detail_with_overrides(self, base: str, override: Optional[PackageOverride]) -> str:
        if override and (override.system_packages or override.system_recipe):
            details = []
            if override.system_packages:
                sys_deps = ", ".join(override.system_packages)
                details.append(f"system packages required: {sys_deps}")
            if override.system_recipe:
                details.append("system recipe steps provided")
            return f"{base} ({'; '.join(details)})"
        return base

    def _record_history(
        self,
        entry: ManifestEntry,
        job: BuildJob,
        override: Optional[PackageOverride],
        *,
        cached: bool,
        recipe_ran: bool = False,
        metadata: Optional[dict] = None,
    ) -> None:
        if not self.history:
            return
        meta = metadata.copy() if metadata else {}
        if override:
            meta["override_env"] = override.env
            meta["system_packages"] = override.system_packages
            meta["system_recipe"] = override.system_recipe
            meta["notes"] = override.notes
        self.history.record_event(
            run_id=self.run_id,
            name=job.name,
            version=job.version,
            python_tag=job.python_tag,
            platform_tag=job.platform_tag,
            status=entry.status,
            source_spec=job.source_spec,
            detail=entry.detail,
            wheel_path=entry.path,
            cached=cached,
            metadata={**meta, "system_recipe_ran": recipe_ran},
        )

    def _variants(self, package_name: str) -> list[BuildVariant]:
        base = [
            BuildVariant(name="default", build_isolation=True, env_patch={}, extra_build_args=[]),
            BuildVariant(name="no_isolation", build_isolation=False, env_patch={}, extra_build_args=[]),
            BuildVariant(
                name="arch_tweak",
                build_isolation=False,
                env_patch={"CFLAGS": "-fno-semantic-interposition"},
                extra_build_args=[],
            ),
        ]
        if self.history:
            rates = self.history.variant_success_rate(package_name)
            if rates:
                base.sort(key=lambda v: -(rates.get(v.name, 0)))
        return base

    def _containerized_command(self, cmd: list[str], env: dict, workdir: Optional[Path], *, cpu: Optional[float] = None, mem: Optional[float] = None) -> list[str]:
        engine = self._container_engine or "docker"
        image = self._container_image
        if not image:
            return cmd
        wrapped_env = []
        for key, val in (env or {}).items():
            wrapped_env.extend(["-e", f"{key}={val}"])
        mounts = [
            "-v",
            f"{self.cache_dir}:{self.cache_dir}",
            "-v",
            f"{self.output_dir}:{self.output_dir}",
        ]
        workdir_arg = ["-w", str(workdir)] if workdir else []
        limits: list[str] = []
        cpu_limit = cpu or self.config.container_cpu
        mem_limit = mem or self.config.container_memory
        if cpu_limit:
            limits += ["--cpus", str(cpu_limit)]
        if mem_limit:
            limits += ["--memory", str(mem_limit)]
        shell_cmd = " ".join(shlex.quote(c) for c in cmd)
        return [engine, "run", "--rm", *limits, *mounts, *wrapped_env, *workdir_arg, image, "sh", "-c", shell_cmd]

    def _apply_suggestion(self, job: BuildJob, hint: str) -> None:
        override = self.config.overrides.get(job.name) or self.config.overrides.get(job.name.lower())
        if not override:
            override = PackageOverride()
            self.config.overrides[job.name] = override
        if "Suggest install:" in hint:
            suggestion = hint.split("Suggest install:")[-1].strip()
            if suggestion not in override.system_packages:
                override.system_packages.append(suggestion)
                LOG.info("Auto-applied suggested system package for %s: %s", job.name, suggestion)
        for step in self._recipe_steps_from_hint(hint):
            if step not in override.system_recipe:
                override.system_recipe.append(step)


def _first_sdist(paths: Iterable[Path]) -> Optional[Path]:
    for path in paths:
        if path.suffix in {".zip", ".gz", ".bz2", ".xz", ".tar"} or path.name.endswith(".tar.gz"):
            return path
    return None


def _extract_sdist(source: Path, destination: Path) -> None:
    if source.suffix == ".zip":
        with ZipFile(source) as zf:
            zf.extractall(destination)
        return
    if source.suffix in {".gz", ".bz2", ".xz", ".tar"} or source.name.endswith(".tar.gz"):
        with tarfile.open(source) as tf:
            tf.extractall(destination)
        return
    raise ValueError(f"Unsupported sdist format: {source}")


    def _hint_from_logs(self, output: str) -> Optional[str]:
        # Catalog driven
        catalog_match = self.hint_catalog.match(output)
        if catalog_match:
            parts = []
            if catalog_match.packages.get("dnf"):
                parts.append("dnf: " + " ".join(catalog_match.packages["dnf"]))
            if catalog_match.packages.get("apt"):
                parts.append("apt: " + " ".join(catalog_match.packages["apt"]))
            suggestion = " | ".join(parts)
            recipes = catalog_match.recipes or {}
            recipe_steps = []
            if recipes.get("dnf"):
                recipe_steps.extend(recipes["dnf"])
            if recipes.get("apt"):
                recipe_steps.extend(recipes["apt"])
            recipe_text = "; ".join(recipe_steps)
            return f"Suggested packages: {suggestion}" + (f" | Recipes: {recipe_text}" if recipe_text else "")

        # Fallback heuristic patterns
        missing_lib = re.search(r"cannot find -l([A-Za-z0-9_\-]+)", output)
        if missing_lib:
            lib = missing_lib.group(1)
            return f"Missing system library lib{lib}"
        missing_header = re.search(r"fatal error: ([A-Za-z0-9_/.\-]+\.h): No such file or directory", output)
        if missing_header:
            header = missing_header.group(1)
            return f"Missing header {header}"
        missing_file = re.search(r"No such file or directory: '([^']+)'", output)
        if missing_file:
            return f"Missing file {missing_file.group(1)} (check build deps)"
        missing_module = re.search(r"ModuleNotFoundError: No module named ['\"]([^'\"]+)['\"]", output)
        if missing_module:
            name = missing_module.group(1)
            return f"Missing Python module {name} (build dependency?)"
        return None

    def _recipe_steps_from_hint(self, hint: str) -> list[str]:
        steps: list[str] = []
        if "Suggested packages:" in hint:
            suggestion = hint.split("Suggested packages:")[-1].strip()
            parts = suggestion.split("|")
            dnf_cmds = []
            apt_cmds = []
            for part in parts:
                part = part.strip()
                if part.startswith("dnf:"):
                    dnf_cmds.extend(part.replace("dnf:", "").strip().split())
                if part.startswith("apt:"):
                    apt_cmds.extend(part.replace("apt:", "").strip().split())
            if dnf_cmds:
                steps.append("dnf install -y " + " ".join(dnf_cmds))
            if apt_cmds:
                steps.append("apt-get update && apt-get install -y " + " ".join(apt_cmds))
        return steps



def _default_image_for_preset(preset: Optional[str]) -> Optional[str]:
    presets = {
        "rocky": "docker.io/rockylinux:9",
        "fedora": "docker.io/fedora:40",
        "ubuntu": "docker.io/ubuntu:22.04",
    }
    if not preset:
        return None
    return presets.get(preset)
