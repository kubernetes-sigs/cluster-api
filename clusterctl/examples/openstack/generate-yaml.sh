#!/bin/bash
set -e

PROVIDERCOMPONENT_TEMPLATE_FILE=provider-components.yaml.template
PROVIDERCOMPONENT_GENERATED_FILE=provider-components.yaml

MACHINE_CONTROLLER_SSH_PUBLIC_FILE=openstack_tmp.pub
MACHINE_CONTROLLER_SSH_PUBLIC=
MACHINE_CONTROLLER_SSH_PRIVATE_FILE=openstack_tmp
MACHINE_CONTROLLER_SSH_PRIVATE=
MACHINE_CONTROLLER_SSH_HOME=~/.ssh/

OVERWRITE=0

SCRIPT=$(basename $0)
while test $# -gt 0; do
        case "$1" in
          -h|--help)
            echo "$SCRIPT - generates input yaml files for Cluster API on openstack"
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

# Get user cloud provider information from input
read -p "Enter your username:" username
read -p "Enter your domainname:" domain_name
read -p "Enter your tenant id:" tenant_id
read -p "Enter region name:" region
read -p "Enter authurl:" authurl
read -s -p "Enter your password:" password
OPENSTACK_CLOUD_CONFIG_PLAIN="user-name: $username\n\
password: $password\n\
domain-name: $domain_name\n\
tenant-id: $tenant_id\n\
region: $region\n\
auth-url: $authurl\n"

# Check if the ssh key already exists. If not, generate and copy to the .ssh dir.
if [ ! -f $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE ]; then
  echo "Generating SSH key files for machine controller."
  # This is needed because GetKubeConfig assumes the key in the home .ssh dir.
  ssh-keygen -t rsa -f $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE  -N ""
fi
MACHINE_CONTROLLER_SSH_PLAIN=clusterapi

OS=$(uname)
if [[ "$OS" =~ "Linux" ]]; then
  OPENSTACK_CLOUD_CONFIG=$(echo -e $OPENSTACK_CLOUD_CONFIG_PLAIN|base64 -w0)
  MACHINE_CONTROLLER_SSH_USER=$(echo -n $MACHINE_CONTROLLER_SSH_PLAIN|base64 -w0)
  MACHINE_CONTROLLER_SSH_PUBLIC=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PUBLIC_FILE|base64 -w0)
  MACHINE_CONTROLLER_SSH_PRIVATE=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE|base64 -w0)
elif [[ "$OS" =~ "Darwin" ]]; then
  OPENSTACK_CLOUD_CONFIG=$(echo -e $OPENSTACK_CLOUD_CONFIG_PLAIN|base64)
  MACHINE_CONTROLLER_SSH_USER=$(echo -n $MACHINE_CONTROLLER_SSH_PLAIN|base64)
  MACHINE_CONTROLLER_SSH_PUBLIC=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PUBLIC_FILE|base64)
  MACHINE_CONTROLLER_SSH_PRIVATE=$(cat $MACHINE_CONTROLLER_SSH_HOME$MACHINE_CONTROLLER_SSH_PRIVATE_FILE|base64)
else
  echo "Unrecognized OS : $OS"
  exit 1
fi

cat $PROVIDERCOMPONENT_TEMPLATE_FILE \
  | sed -e "s/\$OPENSTACK_CLOUD_CONFIG/$OPENSTACK_CLOUD_CONFIG/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_USER/$MACHINE_CONTROLLER_SSH_USER/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PUBLIC/$MACHINE_CONTROLLER_SSH_PUBLIC/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PRIVATE/$MACHINE_CONTROLLER_SSH_PRIVATE/" \
  > $PROVIDERCOMPONENT_GENERATED_FILE

echo "Done generating $PROVIDERCOMPONENT_GENERATED_FILE"
echo "You will still need to generate the cluster.yaml and machines.yaml"

