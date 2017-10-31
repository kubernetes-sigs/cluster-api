package main

// TODO: actually init or join the cluster, templatize token, etc.

const nodeStartupTemplate = `
#!/bin/bash

set -e
set -x

(
apt-get update
apt-get install -y apt-transport-https
apt-key adv --keyserver hkp://keyserver.ubuntu.com --recv-keys F76221572C52609D

cat <<EOF > /etc/apt/sources.list.d/k8s.list
deb [arch=amd64] https://apt.dockerproject.org/repo ubuntu-xenial main
EOF

apt-get update
apt-get install -y docker-engine=1.12.0-0~xenial
systemctl enable docker || true
systemctl start docker || true

curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -

cat <<EOF > /etc/apt/sources.list.d/kubernetes.list
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF
apt-get update
apt-get install -y kubelet kubeadm kubectl kubernetes-cni

echo done.
) 2>&1 | tee /var/log/startup.log
`

const masterStartupTemplate = `
#!/bin/bash

set -e
set -x

(
apt-get update
apt-get install -y apt-transport-https
apt-key adv --keyserver hkp://keyserver.ubuntu.com --recv-keys F76221572C52609D

cat <<EOF > /etc/apt/sources.list.d/k8s.list
deb [arch=amd64] https://apt.dockerproject.org/repo ubuntu-xenial main
EOF

apt-get update
apt-get install -y docker-engine=1.12.0-0~xenial
systemctl enable docker || true
systemctl start docker || true

curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -

cat <<EOF > /etc/apt/sources.list.d/kubernetes.list
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF
apt-get update
apt-get install -y kubelet kubeadm kubectl kubernetes-cni

echo done.
) 2&>1 | tee /var/log/startup.log
`
