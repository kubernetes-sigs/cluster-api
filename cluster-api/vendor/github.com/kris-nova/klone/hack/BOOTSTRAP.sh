#!/usr/bin/env bash
# Copyright © 2017 Kris Nova <kris@nivenly.com>
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
#
#  _  ___
# | |/ / | ___  _ __   ___
# | ' /| |/ _ \| '_ \ / _ \
# | . \| | (_) | | | |  __/
# |_|\_\_|\___/|_| |_|\___|
#
# BOOTSTRAP.sh will bootstrap and run klone on any system
# Usage: BOOTSTRAP.sh <query> <bash:command>

if [ -z "$1" ]; then
    echo "Usage: BOOTSTRAP.sh <query> <command>"
    exit 1
fi
QUERY=${1}

CMD=""
if [ -n "$2" ]; then
    CMD=${2}
fi

cp -r /tmp/klone ~/.klone

exists() {
  command -v "$1" >/dev/null 2>&1
}

VERSION=$(cat ~/.klone/version)
INSTALL_DIR="/usr/local/bin"
BIN_NAME="darwin-amd64"

if [[ "$(uname)" == "Darwin" ]]; then
    echo "[klone]:  Detected architecture [darwin]"
    BIN_NAME="darwin-amd64"
elif [ "$(expr substr $(uname -s) 1 5)" == "Linux" ]; then
   echo "[klone]:  Detected architecture [linux]"
   BIN_NAME="linux-amd64"
elif [ "$(expr substr $(uname -s) 1 10)" == "MINGW64_NT" ]; then
   echo "[klone]:  Detected architecture [win64]"
    https://github.com/kris-nova/klone/releases/download/v${VERSION}/windows-amd64
    BIN_NAME="windows-amd64"
fi
DOWNLOAD_URL="https://github.com/kris-nova/klone/releases/download/v${VERSION}/${BIN_NAME}"

if exists wget; then
    echo "[klone]:  Command exists [wget]"
else


    # --------------------------- apt-get ---------------------------
    if exists apt-get; then
        echo "[klone]:  Downloading [wget]"
        apt-get update  &> /dev/null
        apt-get install -y wget  &> /dev/null
    fi

    # --------------------------- yum ---------------------------
    if exists yum; then
        echo "[klone]:  Downloading [wget]"
        yum install -y wget  &> /dev/null
    fi

    # --------------------------- pacman ---------------------------
    if exists pacman; then
        echo "[klone]:  Downloading [wget]"
        pacman -Syy
        pacman -S --noconfirm wget
    fi

fi

if exists klone; then
    echo "[klone]:  Command exists [klone]"
else
    echo "[klone]:  Downloading [klone]"
    # assume wget
    wget $DOWNLOAD_URL &> /dev/null
    chmod +x $BIN_NAME
    mv $BIN_NAME $INSTALL_DIR/klone
    PATH=$PATH:$INSTALL_DIR
fi

cd ~
klone ${QUERY}
if [ -n "$CMD" ]; then
    eval ${CMD}
fi


