#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

INSTALL_DIR="/opt/pypy"
PYPY_PATH="https://bitbucket.org/pypy/pypy/downloads/pypy-5.1.0-linux64.tar.bz2"
PYPY_HASH="0e8913351d043a50740b98cb89d99852b8bd6d11225a41c8abfc0baf7084cbf6"

install() {
  rm -rf "${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}"
  curl -sL --fail -o "${INSTALL_DIR}/pypy.tar.bz2" "${PYPY_PATH}"
  echo "${PYPY_HASH} ${INSTALL_DIR}/pypy.tar.bz2" | sha256sum -c
  tar xvfj "${INSTALL_DIR}/pypy.tar.bz2" --strip-components=1 -C "${INSTALL_DIR}"
}

if [[ ! -f "${INSTALL_DIR}/bin/pypy" ]]; then
  install
fi
