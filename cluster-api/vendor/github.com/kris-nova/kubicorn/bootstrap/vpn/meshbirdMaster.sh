#!/usr/bin/env bash
set -e
cd ~

# VPN Mesh
curl http://meshbird.com/install.sh | sh
meshbird new &> /tmp/logkey
MESHBIRD_KEY=$(cat /tmp/logkey | cut -d " " -f 5)
echo $MESHBIRD_KEY > /tmp/.key
export MESHBIRD_KEY=$MESHBIRD_KEY
meshbird join &
