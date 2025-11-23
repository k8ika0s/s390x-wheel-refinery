from pathlib import Path

from s390x_wheel_refinery.config import build_config, UpgradeStrategy


def test_build_config_merges_cli_and_file(tmp_path: Path):
    cfg_path = tmp_path / "config.toml"
    cfg_path.write_text(
        """
[refinery]
target_python = "3.11"
upgrade_strategy = "eager"
[refinery.index]
index_url = "https://example.com/simple"
"""
    )
    cfg = build_config(
        target_python="3.10",
        upgrade_strategy=None,
        config_file=cfg_path,
        index_url=None,
        extra_index_urls=["https://extra/simple"],
    )
    assert cfg.target_python == "3.10"
    assert cfg.upgrade_strategy == UpgradeStrategy.EAGER
    assert cfg.index.index_url == "https://example.com/simple"
    assert cfg.index.extra_index_urls == ["https://extra/simple"]


def test_python_tag_from_version():
    cfg = build_config(target_python="3.11")
    assert cfg.python_tag == "cp311"
