from s390x_wheel_refinery.hints import HintCatalog


def test_hint_catalog_matches_missing_lib(tmp_path):
    catalog = HintCatalog(tmp_path / "nonexistent.yaml")
    assert catalog.match("cannot find -lssl") is None

    from pathlib import Path

    # Use real catalog
    real_catalog = HintCatalog(Path("data/hints.yaml"))
    match = real_catalog.match("fatal error: numpy/arrayobject.h")
    assert match is not None
    assert "apt" in match.packages
