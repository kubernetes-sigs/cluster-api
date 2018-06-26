#!/bin/bash

# Copyright 2018 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail

PROVIDERCOMPONENT_TEMPLATE_FILE=provider-components.yaml.template
PROVIDERCOMPONENT_GENERATED_FILE=provider-components.yaml

MACHINE_CONTROLLER_SSH_PUBLIC_FILE=vsphere_tmp.pub
MACHINE_CONTROLLER_SSH_PUBLIC=
MACHINE_CONTROLLER_SSH_PRIVATE_FILE=vsphere_tmp
MACHINE_CONTROLLER_SSH_PRIVATE=
MACHINE_CONTROLLER_SSH_HOME=~/.ssh/

OVERWRITE=0

SCRIPT=$(basename $0)
while test $# -gt 0; do
        case "$1" in
          -h|--help)
            echo "$SCRIPT - generates input yaml files for Cluster API on vSphere"
            echo " "
            echo "$SCRIPT [options]"
            echo " "
            echo "options:"
            echo "-h, --help                show brief help"
            echo "-f, --force-overwrite     if file to be generated already exists, force script to overwrite it"
            exit 0
            ;;
          -f)
            OVERWRITE=1
            shift
            ;;
          --force-overwrite)
            OVERWRITE=1
            shift
            ;;
          *)
            break
            ;;
        esac
done

if [ $OVERWRITE -ne 1 ] && [ -f $PROVIDERCOMPONENT_GENERATED_FILE ]; then
  echo "File $PROVIDERCOMPONENT_GENERATED_FILE already exists. Delete it manually before running this script."
  exit 1
fi

# Check if the ssh key already exists. If not, generate and copy to the .ssh dir.
if [ ! -f $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE ]; then
  echo "Generating SSH key files for machine controller."
  # This is needed because GetKubeConfig assumes the key in the home .ssh dir.
  ssh-keygen -t rsa -f $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE  -N ""
fi

# By default, linux wraps base64 output every 76 cols, so we use 'tr -d' to remove whitespaces.
# Note 'base64 -w0' doesn't work on Mac OS X, which has different flags.
MACHINE_CONTROLLER_SSH_PUBLIC=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PUBLIC_FILE | base64 | tr -d '\r\n')
MACHINE_CONTROLLER_SSH_PRIVATE=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE | base64 | tr -d '\r\n')

cat $PROVIDERCOMPONENT_TEMPLATE_FILE \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PUBLIC/$MACHINE_CONTROLLER_SSH_PUBLIC/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PRIVATE/$MACHINE_CONTROLLER_SSH_PRIVATE/" \
  > $PROVIDERCOMPONENT_GENERATED_FILE

echo "Done generating $PROVIDERCOMPONENT_GENERATED_FILE"
echo "You will still need to generate the cluster.yaml,  machines.yaml, and addons.yaml (if needed) configuration files"
