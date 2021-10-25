#!/bin/bash

cd "$(dirname "$0")" || exit

set -euo pipefail

OUTPUT="$(tilt ci)"

if ! (echo "$OUTPUT" | grep -q abcdef); then
  echo "did not find string 'abcdef' in output"
  echo "output:"
  echo
  echo "$OUTPUT"
  exit 1
fi
