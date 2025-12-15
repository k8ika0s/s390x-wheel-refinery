#!/usr/bin/env bash
# Central pinset for build-pack recipes.
#
# Intentionally boring: recipes source this file and then use:
#   PACK_VERSION="$FOO_VERSION"
#   PACK_SOURCE_URL="$FOO_URL"
#   PACK_SOURCE_SHA256="$FOO_SHA256"
#
# You should fill in the *_SHA256 values from upstream release checksums
# (or your own mirrored artifact store). By default, recipes will FAIL if
# sha256 is missing (unless ALLOW_MISSING_SHA256=1).

# Common
: "${PACK_POLICY:=manylinux2014_s390x}"

# ---- Core ----
: "${ZLIB_VERSION:=1.3.1}"
: "${ZLIB_SOURCE_URL:=https://zlib.net/zlib-${ZLIB_VERSION}.tar.gz}"
: "${ZLIB_SOURCE_SHA256:=}"

: "${OPENSSL_VERSION:=3.0.14}"
: "${OPENSSL_SOURCE_URL:=https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz}"
: "${OPENSSL_SOURCE_SHA256:=}"

: "${LIBFFI_VERSION:=3.4.6}"
: "${LIBFFI_SOURCE_URL:=https://github.com/libffi/libffi/releases/download/v${LIBFFI_VERSION}/libffi-${LIBFFI_VERSION}.tar.gz}"
: "${LIBFFI_SOURCE_SHA256:=}"

# ---- Compression ----
: "${BZIP2_VERSION:=1.0.8}"
: "${BZIP2_SOURCE_URL:=https://sourceware.org/pub/bzip2/bzip2-${BZIP2_VERSION}.tar.gz}"
: "${BZIP2_SOURCE_SHA256:=}"

: "${XZ_VERSION:=5.6.2}"
: "${XZ_SOURCE_URL:=https://tukaani.org/xz/xz-${XZ_VERSION}.tar.gz}"
: "${XZ_SOURCE_SHA256:=}"

: "${ZSTD_VERSION:=1.5.6}"
: "${ZSTD_SOURCE_URL:=https://github.com/facebook/zstd/releases/download/v${ZSTD_VERSION}/zstd-${ZSTD_VERSION}.tar.gz}"
: "${ZSTD_SOURCE_SHA256:=}"

# ---- DB / XML ----
: "${SQLITE_VERSION:=3450300}"  # sqlite "autoconf" numeric version, not 3.45.3
: "${SQLITE_SOURCE_URL:=https://www.sqlite.org/2024/sqlite-autoconf-${SQLITE_VERSION}.tar.gz}"
: "${SQLITE_SOURCE_SHA256:=}"

: "${LIBXML2_VERSION:=2.12.7}"
: "${LIBXML2_SOURCE_URL:=https://download.gnome.org/sources/libxml2/2.12/libxml2-${LIBXML2_VERSION}.tar.xz}"
: "${LIBXML2_SOURCE_SHA256:=}"

: "${LIBXSLT_VERSION:=1.1.39}"
: "${LIBXSLT_SOURCE_URL:=https://download.gnome.org/sources/libxslt/1.1/libxslt-${LIBXSLT_VERSION}.tar.xz}"
: "${LIBXSLT_SOURCE_SHA256:=}"

# ---- Imaging ----
: "${LIBJPEG_TURBO_VERSION:=3.0.3}"
: "${LIBJPEG_TURBO_SOURCE_URL:=https://github.com/libjpeg-turbo/libjpeg-turbo/archive/refs/tags/${LIBJPEG_TURBO_VERSION}.tar.gz}"
: "${LIBJPEG_TURBO_SOURCE_SHA256:=}"

: "${LIBPNG_VERSION:=1.6.43}"
: "${LIBPNG_SOURCE_URL:=https://download.sourceforge.net/libpng/libpng-${LIBPNG_VERSION}.tar.xz}"
: "${LIBPNG_SOURCE_SHA256:=}"

: "${FREETYPE_VERSION:=2.13.2}"
: "${FREETYPE_SOURCE_URL:=https://download.savannah.gnu.org/releases/freetype/freetype-${FREETYPE_VERSION}.tar.xz}"
: "${FREETYPE_SOURCE_SHA256:=}"

# ---- Math ----
: "${OPENBLAS_VERSION:=0.3.27}"
: "${OPENBLAS_SOURCE_URL:=https://github.com/OpenMathLib/OpenBLAS/releases/download/v${OPENBLAS_VERSION}/OpenBLAS-${OPENBLAS_VERSION}.tar.gz}"
: "${OPENBLAS_SOURCE_SHA256:=}"

# ---- Build tools ----
: "${PKGCONF_VERSION:=2.2.0}"
: "${PKGCONF_SOURCE_URL:=https://distfiles.ariadne.space/pkgconf/pkgconf-${PKGCONF_VERSION}.tar.xz}"
: "${PKGCONF_SOURCE_SHA256:=}"

: "${CMAKE_VERSION:=3.30.1}"
: "${CMAKE_SOURCE_URL:=https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}.tar.gz}"
: "${CMAKE_SOURCE_SHA256:=}"

: "${NINJA_VERSION:=1.12.1}"
: "${NINJA_SOURCE_URL:=https://github.com/ninja-build/ninja/archive/refs/tags/v${NINJA_VERSION}.tar.gz}"
: "${NINJA_SOURCE_SHA256:=}"

: "${RUST_VERSION:=1.82.0}"
: "${RUST_TARGET_TRIPLE:=s390x-unknown-linux-gnu}"
: "${RUST_SOURCE_URL:=https://static.rust-lang.org/dist/rust-${RUST_VERSION}-${RUST_TARGET_TRIPLE}.tar.xz}"
: "${RUST_SOURCE_SHA256:=}"

# ---- CPython runtimes ----
# Pin to the exact CPython patch versions you ship.
: "${PY310_VERSION:=3.10.14}"
: "${PY311_VERSION:=3.11.9}"
: "${PY312_VERSION:=3.12.4}"

# CPython sources come from python.org.
: "${PYTHON_SOURCE_URL_BASE:=https://www.python.org/ftp/python}"
: "${PY310_SOURCE_URL:=${PYTHON_SOURCE_URL_BASE}/${PY310_VERSION}/Python-${PY310_VERSION}.tgz}"
: "${PY311_SOURCE_URL:=${PYTHON_SOURCE_URL_BASE}/${PY311_VERSION}/Python-${PY311_VERSION}.tgz}"
: "${PY312_SOURCE_URL:=${PYTHON_SOURCE_URL_BASE}/${PY312_VERSION}/Python-${PY312_VERSION}.tgz}"

: "${PY310_SOURCE_SHA256:=}"
: "${PY311_SOURCE_SHA256:=}"
: "${PY312_SOURCE_SHA256:=}"
