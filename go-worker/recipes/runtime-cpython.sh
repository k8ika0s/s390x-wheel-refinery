#!/bin/sh
set -eu
PYVER=${PYTHON_VERSION:-3.11.8}
PREFIX=${RUNTIME_OUTPUT:-/tmp/runtime-out}
SRCURL=${PYTHON_SRC_URL:-"https://www.python.org/ftp/python/${PYVER}/Python-${PYVER}.tgz"}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"
echo "Downloading CPython ${PYVER} from ${SRCURL}"
wget -q "${SRCURL}" -O python.tgz
tar xf python.tgz
cd Python-${PYVER}
./configure --prefix="$PREFIX" --enable-shared --with-ensurepip=install
make -j"$(nproc)"
make install
# Strip to reduce size
test -x "$PREFIX/bin/python3" && strip "$PREFIX/bin/python3" || true
echo "Runtime built at $PREFIX"
