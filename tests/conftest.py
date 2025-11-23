import shutil
import zipfile
from pathlib import Path


def write_dummy_wheel(
    tmpdir: Path,
    name: str,
    version: str,
    python_tag: str = "py3",
    abi_tag: str = "none",
    platform_tag: str = "any",
    requires: list[str] | None = None,
) -> Path:
    wheel_name = f"{name}-{version}-{python_tag}-{abi_tag}-{platform_tag}.whl"
    wheel_path = tmpdir / wheel_name
    dist_info = f"{name.replace('-', '_')}-{version}.dist-info"
    metadata_path = f"{dist_info}/METADATA"
    requires = requires or []
    metadata = "\n".join([f"Name: {name}", f"Version: {version}", *(f"Requires-Dist: {r}" for r in requires)])
    with zipfile.ZipFile(wheel_path, "w") as zf:
        zf.writestr(metadata_path, metadata)
    return wheel_path


def clean_dir(path: Path):
    if path.exists():
        shutil.rmtree(path)
