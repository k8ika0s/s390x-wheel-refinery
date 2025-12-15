# s390x build-pack recipe book (starter)

This is a **starter set** of build-pack recipes designed for a wheel-build "refinery" style system.

## Contract each recipe follows

- The worker sets `PACK_OUTPUT` to an existing, **empty** directory.
- Recipes install into `${PACK_OUTPUT}/usr/local` by default (override with `PACK_PREFIX_REL`).
- Recipes should be deterministic-ish: stable flags, no timestamps, minimal host leakage.
- Recipes write `${PACK_OUTPUT}/manifest.json`.

## Shared knobs

- `PACK_POLICY` (default: `manylinux2014_s390x`)
- `DEPS_PREFIXES` — colon-separated list of dependency prefixes already mounted, e.g.
  `/opt/packs/<digest>/usr/local:/opt/packs/<digest2>/usr/local`
- `SOURCES_DIR` — optional cache directory for downloaded tarballs
- `JOBS` — parallelism (defaults to `nproc`)
- `KEEP_STATIC=1` — keep `.a` files
- `KEEP_DOCS=1` — keep docs/manpages
- `ALLOW_MISSING_SHA256=1` — skip sha verification (not recommended)

## Pinning versions and checksums

Edit `versions.sh`:
- Set `*_VERSION`, `*_SOURCE_URL`, and **fill in** `*_SOURCE_SHA256`.
- Recipes will fail if sha256 is missing (unless `ALLOW_MISSING_SHA256=1`).

## Example: building zlib into a staging directory

```bash
export PACK_OUTPUT="$(mktemp -d)"
export SOURCES_DIR="$PWD/.sources"
./recipes/zlib.sh
tree "$PACK_OUTPUT"
```

## Dependency order (typical)

- pkgconf (pkg-config provider)
- zlib
- xz
- bzip2
- openssl (optionally uses zlib)
- libffi
- sqlite
- libxml2 -> libxslt
- jpeg (libjpeg-turbo), libpng -> freetype
- openblas
- cpython310/cpython311/cpython312 (needs openssl + compression + ffi + sqlite)

