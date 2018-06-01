#!/bin/sh
set -e

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
  echo File $PROVIDERCOMPONENT_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ ! -f $MACHINE_CONTROLLER_SSH_PRIVATE_FILE ]; then
  echo Generate SSH key files fo machine controller
  ssh-keygen -t rsa -f $MACHINE_CONTROLLER_SSH_PRIVATE_FILE  -N ""
fi

# Copy file to home ssh directory till using vsphere GetIP logic that
# does not assume the file at this location
cp $MACHINE_CONTROLLER_SSH_PUBLIC_FILE $MACHINE_CONTROLLER_SSH_HOME
cp $MACHINE_CONTROLLER_SSH_PRIVATE_FILE $MACHINE_CONTROLLER_SSH_HOME

MACHINE_CONTROLLER_SSH_PUBLIC=$(cat $MACHINE_CONTROLLER_SSH_PUBLIC_FILE|base64 -w0)
MACHINE_CONTROLLER_SSH_PRIVATE=$(cat $MACHINE_CONTROLLER_SSH_PRIVATE_FILE|base64 -w0)

cat $PROVIDERCOMPONENT_TEMPLATE_FILE \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PUBLIC/$MACHINE_CONTROLLER_SSH_PUBLIC/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PRIVATE/$MACHINE_CONTROLLER_SSH_PRIVATE/" \
  > $PROVIDERCOMPONENT_GENERATED_FILE