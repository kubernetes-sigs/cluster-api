#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script downloads and install the mdBook binary.

set -o errexit
set -o nounset
set -o pipefail

VERSION=${1}
OUTPUT_PATH=${2}

# Ensure the output folder exists
mkdir -p "${OUTPUT_PATH}"

# Get what release to download
RELEASE_NAME=""
case "$OSTYPE" in
  darwin*) RELEASE_NAME="x86_64-apple-darwin.tar.gz"  ;;
  linux*)  RELEASE_NAME="x86_64-unknown-linux-gnu.tar.gz" ;;
#  msys*)    echo "WINDOWS" ;;
  *)        echo "No mdBook release available for: $OSTYPE" && exit 1;;
esac

# mdBook releases do not publish checksums or signatures, so known-good sha256
# digests are pinned here and must be updated whenever MDBOOK_VERSION is bumped.
EXPECTED_SHA256=""
case "${VERSION}-${RELEASE_NAME}" in
  v0.4.11-x86_64-apple-darwin.tar.gz)       EXPECTED_SHA256="16615a2b4b5e623f35d27c24fd6651b8e80cdcb315176c3c8feba07161442811" ;;
  v0.4.11-x86_64-unknown-linux-gnu.tar.gz)  EXPECTED_SHA256="d26c32fa09e0199ffa30705beb05a16b17a2fb2e96977de277f96695f6185049" ;;
  *) echo "No known sha256 digest for mdbook-${VERSION}-${RELEASE_NAME}, refusing to install an unverified binary" && exit 1 ;;
esac

# Download the mdBook release, verify its digest, then extract it
ARCHIVE="$(mktemp -t mdbook.XXXXXX).tar.gz"
trap 'rm -f "${ARCHIVE}"' EXIT

curl -L -o "${ARCHIVE}" "https://github.com/rust-lang/mdBook/releases/download/${VERSION}/mdbook-${VERSION}-${RELEASE_NAME}"

ACTUAL_SHA256="$(shasum -a 256 "${ARCHIVE}" | awk '{print $1}')"
if [[ "${ACTUAL_SHA256}" != "${EXPECTED_SHA256}" ]]; then
  echo "sha256 mismatch for mdbook-${VERSION}-${RELEASE_NAME}: expected ${EXPECTED_SHA256}, got ${ACTUAL_SHA256}"
  exit 1
fi

tar -xvz -C "${OUTPUT_PATH}" -f "${ARCHIVE}"
