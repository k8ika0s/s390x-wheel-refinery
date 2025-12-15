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
: "${ZLIB_SOURCE_SHA256:=9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23}"

: "${OPENSSL_VERSION:=3.0.14}"
: "${OPENSSL_SOURCE_URL:=https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz}"
: "${OPENSSL_SOURCE_SHA256:=eeca035d4dd4e84fc25846d952da6297484afa0650a6f84c682e39df3a4123ca}"

: "${LIBFFI_VERSION:=3.4.6}"
: "${LIBFFI_SOURCE_URL:=https://github.com/libffi/libffi/releases/download/v${LIBFFI_VERSION}/libffi-${LIBFFI_VERSION}.tar.gz}"
: "${LIBFFI_SOURCE_SHA256:=b0dea9df23c863a7a50e825440f3ebffabd65df1497108e5d437747843895a4e}"

# ---- Compression ----
: "${BZIP2_VERSION:=1.0.8}"
: "${BZIP2_SOURCE_URL:=https://sourceware.org/pub/bzip2/bzip2-${BZIP2_VERSION}.tar.gz}"
: "${BZIP2_SOURCE_SHA256:=ab5a03176ee106d3f0fa90e381da478ddae405918153cca248e682cd0c4a2269}"

: "${XZ_VERSION:=5.6.2}"
: "${XZ_SOURCE_URL:=https://tukaani.org/xz/xz-${XZ_VERSION}.tar.gz}"
: "${XZ_SOURCE_SHA256:=8bfd20c0e1d86f0402f2497cfa71c6ab62d4cd35fd704276e3140bfb71414519}"

: "${ZSTD_VERSION:=1.5.6}"
: "${ZSTD_SOURCE_URL:=https://github.com/facebook/zstd/releases/download/v${ZSTD_VERSION}/zstd-${ZSTD_VERSION}.tar.gz}"
: "${ZSTD_SOURCE_SHA256:=8c29e06cf42aacc1eafc4077ae2ec6c6fcb96a626157e0593d5e82a34fd403c1}"

# ---- DB / XML ----
: "${SQLITE_VERSION:=3450300}"  # sqlite "autoconf" numeric version, not 3.45.3
: "${SQLITE_SOURCE_URL:=https://www.sqlite.org/2024/sqlite-autoconf-${SQLITE_VERSION}.tar.gz}"
: "${SQLITE_SOURCE_SHA256:=b2809ca53124c19c60f42bf627736eae011afdcc205bb48270a5ee9a38191531}"

: "${LIBXML2_VERSION:=2.12.7}"
: "${LIBXML2_SOURCE_URL:=https://download.gnome.org/sources/libxml2/2.12/libxml2-${LIBXML2_VERSION}.tar.xz}"
: "${LIBXML2_SOURCE_SHA256:=24ae78ff1363a973e6d8beba941a7945da2ac056e19b53956aeb6927fd6cfb56}"

: "${LIBXSLT_VERSION:=1.1.39}"
: "${LIBXSLT_SOURCE_URL:=https://download.gnome.org/sources/libxslt/1.1/libxslt-${LIBXSLT_VERSION}.tar.xz}"
: "${LIBXSLT_SOURCE_SHA256:=2a20ad621148339b0759c4d4e96719362dee64c9a096dbba625ba053846349f0}"

# ---- Imaging ----
: "${LIBJPEG_TURBO_VERSION:=3.0.3}"
: "${LIBJPEG_TURBO_SOURCE_URL:=https://github.com/libjpeg-turbo/libjpeg-turbo/archive/refs/tags/${LIBJPEG_TURBO_VERSION}.tar.gz}"
: "${LIBJPEG_TURBO_SOURCE_SHA256:=a649205a90e39a548863a3614a9576a3fb4465f8e8e66d54999f127957c25b21}"

: "${LIBPNG_VERSION:=1.6.43}"
: "${LIBPNG_SOURCE_URL:=https://download.sourceforge.net/libpng/libpng-${LIBPNG_VERSION}.tar.xz}"
: "${LIBPNG_SOURCE_SHA256:=6a5ca0652392a2d7c9db2ae5b40210843c0bbc081cbd410825ab00cc59f14a6c}"

: "${FREETYPE_VERSION:=2.13.2}"
: "${FREETYPE_SOURCE_URL:=https://download.savannah.gnu.org/releases/freetype/freetype-${FREETYPE_VERSION}.tar.xz}"
: "${FREETYPE_SOURCE_SHA256:=12991c4e55c506dd7f9b765933e62fd2be2e06d421505d7950a132e4f1bb484d}"

# ---- Math ----
: "${OPENBLAS_VERSION:=0.3.27}"
: "${OPENBLAS_SOURCE_URL:=https://github.com/OpenMathLib/OpenBLAS/releases/download/v${OPENBLAS_VERSION}/OpenBLAS-${OPENBLAS_VERSION}.tar.gz}"
: "${OPENBLAS_SOURCE_SHA256:=aa2d68b1564fe2b13bc292672608e9cdeeeb6dc34995512e65c3b10f4599e897}"

# ---- Build tools ----
: "${PKGCONF_VERSION:=2.2.0}"
: "${PKGCONF_SOURCE_URL:=https://distfiles.ariadne.space/pkgconf/pkgconf-${PKGCONF_VERSION}.tar.xz}"
: "${PKGCONF_SOURCE_SHA256:=b06ff63a83536aa8c2f6422fa80ad45e4833f590266feb14eaddfe1d4c853c69}"

: "${CMAKE_VERSION:=3.30.1}"
: "${CMAKE_SOURCE_URL:=https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}.tar.gz}"
: "${CMAKE_SOURCE_SHA256:=df9b3c53e3ce84c3c1b7c253e5ceff7d8d1f084ff0673d048f260e04ccb346e1}"

: "${NINJA_VERSION:=1.12.1}"
: "${NINJA_SOURCE_URL:=https://github.com/ninja-build/ninja/archive/refs/tags/v${NINJA_VERSION}.tar.gz}"
: "${NINJA_SOURCE_SHA256:=821bdff48a3f683bc4bb3b6f0b5fe7b2d647cf65d52aeb63328c91a6c6df285a}"

: "${RUST_VERSION:=1.82.0}"
: "${RUST_TARGET_TRIPLE:=s390x-unknown-linux-gnu}"
: "${RUST_SOURCE_URL:=https://static.rust-lang.org/dist/rust-${RUST_VERSION}-${RUST_TARGET_TRIPLE}.tar.xz}"
: "${RUST_SOURCE_SHA256:=71428fab3cf18cfe4b4486a11d292ec157fe8b0c904fb4fae34db6539144c286}"

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

: "${PY310_SOURCE_SHA256:=cefea32d3be89c02436711c95a45c7f8e880105514b78680c14fe76f5709a0f6}"
: "${PY311_SOURCE_SHA256:=e7de3240a8bc2b1e1ba5c81bf943f06861ff494b69fda990ce2722a504c6153d}"
: "${PY312_SOURCE_SHA256:=01b3c1c082196f3b33168d344a9c85fb07bfe0e7ecfe77fee4443420d1ce2ad9}"
