#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source /opt/ansible/hacking/env-setup

ROLE="${1}"
INVENTORY_FILE="/host_inventory"
HOST_ROOT="/host_root"

cat <<EOF > "${INVENTORY_FILE}"
[${ROLE}]
${HOST_ROOT}
EOF

ansible-playbook -i "${INVENTORY_FILE}" --connection=chroot /opt/playbooks/install.yml
