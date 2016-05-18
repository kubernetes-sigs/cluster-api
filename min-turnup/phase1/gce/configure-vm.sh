#! /bin/bash

set -o errexit
set -o pipefail
set -o nounset

ROLE=$(curl \
  -H "Metadata-Flavor: Google" \
  "metadata/computeMetadata/v1/instance/attributes/k8s-role")

mkdir -p /etc/systemd/system/docker.service.d/
cat <<EOF > /etc/systemd/system/docker.service.d/clear_mount_propagtion_flags.conf
[Service]
MountFlags=shared
EOF

mkdir -p /etc/kubernetes/
curl -H 'Metadata-Flavor:Google' \
  "metadata/computeMetadata/v1/instance/attributes/k8s-config" \
  -o /etc/kubernetes/k8s_config.json

#TODO: restrict by role
mkdir -p /srv/kubernetes
for bundle in root kubelet apiserver; do
  gsutil cp "gs://mikedanese-k8s-kube-deploy-k-0/crypto/${bundle}.tar" - \
    | sudo tar xv -C /srv/kubernetes
done;

curl -sSL https://get.docker.com/ | sh
apt-get install bzip2
systemctl start docker || true

docker run \
  --net=host \
  -v /:/host_root \
  -v /etc/kubernetes/k8s_config.json:/opt/playbooks/config.json:ro \
  gcr.io/mikedanese-k8s/install-k8s:v1 \
  /opt/do_role.sh "${ROLE}"
