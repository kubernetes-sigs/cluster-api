#!/usr/bin/env bash
set -e
cd ~

# ------------------------------------------------------------------------------------------------------------------------
# These values are injected into the script. We are explicitly not using a templating language to inject the values
# as to encourage the user to limit their use of templating logic in these files. By design all injected values should
# be able to be set at runtime, and the shell script real work. If you need conditional logic, write it in bash
# or make another shell script.
#
#
MESHBIRD_KEY="INJECTEDMESHKEY"
# ------------------------------------------------------------------------------------------------------------------------

# VPN Mesh
curl http://meshbird.com/install.sh | sh
export MESHBIRD_KEY=$MESHBIRD_KEY
meshbird join &
