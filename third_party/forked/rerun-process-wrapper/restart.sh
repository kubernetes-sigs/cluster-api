#!/bin/sh
#
# A helper script to implement restart_container when the docker runtime isn't available.
#
# Usage:
#   Copy start.sh and restart.sh to your container working dir.
#
#   Make your container entrypoint:
#   ./start.sh path-to-binary [args]
#
#   To restart the container:
#   ./restart.sh

set -u

touch restart.txt
PID="$(cat process.txt)"
if [ $? -ne 0 ]; then
  echo "unable to read process.txt. was your process started with start.sh?"
  exit 1
fi
kill "$PID"
