/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package google

import (
	"fmt"
	"strings"
)

func sanitizeMasterIP(ip string) string {
	s := strings.TrimPrefix(ip, "https://")
	parts := strings.Split(s, ":")
	return parts[0]
}

func nodeStartupScript(kubeadmToken, masterIP, machineName, kubeletVersion string) string {
	mip := sanitizeMasterIP(masterIP)
	return fmt.Sprintf(nodeStartupTemplate, kubeadmToken, mip, machineName, kubeletVersion)
}

func masterStartupScript(kubeadmToken, port string) string {
	return fmt.Sprintf(masterStartupTemplate, kubeadmToken, port)
}

const nodeStartupTemplate = `
#!/bin/bash

set -e
set -x

(
TOKEN=%s
MASTER=%s
MACHINE=%s
KUBELET_VERSION=%s-00

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
apt-get install -y kubelet=${KUBELET_VERSION} kubeadm=${KUBELET_VERSION} kubectl=${KUBELET_VERSION}

kubeadm join --token "${TOKEN}" "${MASTER}:443" --skip-preflight-checks

for tries in $(seq 1 60); do
	kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node $(hostname) machine=${MACHINE} && break
	sleep 1
done

echo done.
) 2>&1 | tee /var/log/startup.log
`

// TODO: actually init the cluster, templatize token, etc.
const masterStartupTemplate = `
#!/bin/bash

set -e
set -x

(
TOKEN=%s
PORT=%s
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
touch /etc/apt/sources.list.d/kubernetes.list
sh -c 'echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list'

apt-get update -y
apt-get install -y \
    socat \
    ebtables \
    docker.io \
    apt-transport-https \
    kubelet \
    kubeadm=1.7.0-00 \
    cloud-utils

systemctl enable docker
systemctl start docker
` +
	"PRIVATEIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/ip\"`" + `
echo $PRIVATEIP > /tmp/.ip
` +
	"PUBLICIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip\"`" + `

kubeadm reset
kubeadm init --apiserver-bind-port ${PORT} --token ${TOKEN}  --apiserver-advertise-address ${PUBLICIP} --apiserver-cert-extra-sans ${PUBLICIP} ${PRIVATEIP}

kubectl apply \
  -f http://docs.projectcalico.org/v2.3/getting-started/kubernetes/installation/hosted/kubeadm/1.6/calico.yaml \
  --kubeconfig /etc/kubernetes/admin.conf

echo done.
) 2>&1 | tee /var/log/startup.log
`
