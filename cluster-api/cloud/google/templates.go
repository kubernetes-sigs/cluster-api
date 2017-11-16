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

import "fmt"

func nodeStartupScript(kubeadmToken, master, machineName, kubeletVersion, dnsDomain, serviceCIDR string) string {
	return fmt.Sprintf(nodeStartupTemplate, kubeadmToken, master, machineName, kubeletVersion, dnsDomain, serviceCIDR)
}

func masterStartupScript(kubeadmToken, port, machineName, kubeletVersion, controlPlaneVersion, dnsDomain, podCIDR, serviceCIDR string) string {
	return fmt.Sprintf(masterStartupTemplate, kubeadmToken, port, machineName, kubeletVersion, controlPlaneVersion, dnsDomain, podCIDR, serviceCIDR)
}

const nodeStartupTemplate = `#!/bin/bash

set -e
set -x

(
TOKEN=%s
MASTER=%s
MACHINE=%s
KUBELET_VERSION=%s-00
CLUSTER_DNS_DOMAIN=%s
SERVICE_CIDR=%s

apt-get update
apt-get install -y apt-transport-https prips
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

# kubeadm uses 10th IP as DNS server
CLUSTER_DNS_SERVER=$(prips ${SERVICE_CIDR} | head -n 11 | tail -n 1)

sed -i "s/KUBELET_DNS_ARGS=[^\"]*/KUBELET_DNS_ARGS=--cluster-dns=${CLUSTER_DNS_SERVER} --cluster-domain=${CLUSTER_DNS_DOMAIN}/" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
systemctl restart kubelet.service

kubeadm join --token "${TOKEN}" "${MASTER}" --skip-preflight-checks

for tries in $(seq 1 60); do
	kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node $(hostname) machine=${MACHINE} && break
	sleep 1
done

echo done.
) 2>&1 | tee /var/log/startup.log
`

// TODO: actually init the cluster, templatize token, etc.
const masterStartupTemplate = `#!/bin/bash

set -e
set -x

(
TOKEN=%s
PORT=%s
MACHINE=%s
KUBELET_VERSION=%s
CONTROL_PLANE_VERSION=%s
CLUSTER_DNS_DOMAIN=%s
POD_CIDR=%s
SERVICE_CIDR=%s
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
touch /etc/apt/sources.list.d/kubernetes.list
sh -c 'echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list'

apt-get update -y
apt-get install -y \
    socat \
    ebtables \
    docker.io \
    apt-transport-https \
    kubelet=${KUBELET_VERSION}-00 \
    kubeadm=${KUBELET_VERSION}-00 \
    cloud-utils \
    prips

# kubeadm uses 10th IP as DNS server
CLUSTER_DNS_SERVER=$(prips ${SERVICE_CIDR} | head -n 11 | tail -n 1)

systemctl enable docker
systemctl start docker
sed -i "s/KUBELET_DNS_ARGS=[^\"]*/KUBELET_DNS_ARGS=--cluster-dns=${CLUSTER_DNS_SERVER} --cluster-domain=${CLUSTER_DNS_DOMAIN}/" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
systemctl restart kubelet.service
` +
	"PRIVATEIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/ip\"`" + `
echo $PRIVATEIP > /tmp/.ip
` +
	"PUBLICIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip\"`" + `

KUBEADM_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
ARCH=amd64
curl -sSL https://dl.k8s.io/release/${KUBEADM_VERSION}/bin/linux/${ARCH}/kubeadm > /usr/bin/kubeadm
chmod a+rx /usr/bin/kubeadm

kubeadm reset
kubeadm init --apiserver-bind-port ${PORT} --token ${TOKEN} --kubernetes-version v${CONTROL_PLANE_VERSION} --apiserver-advertise-address ${PUBLICIP} --apiserver-cert-extra-sans ${PUBLICIP} ${PRIVATEIP} --service-cidr ${SERVICE_CIDR}

for tries in $(seq 1 60); do
	kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node $(hostname) machine=${MACHINE} && break
	sleep 1
done

# download Calico manifest and pick a random service IP in the service CIDR
CALICO=$(curl --retry 5 -sL https://docs.projectcalico.org/v2.3/getting-started/kubernetes/installation/hosted/kubeadm/1.6/calico.yaml)
CALICOIP=$(prips ${SERVICE_CIDR} | sed '1d;11d;$d' | shuf -n 1) # random IP excluding network, broadcast, and DNS server addresses
CALICO=${CALICO//10\.96\.232\.136/${CALICOIP}} # replace etcd cluster IP
CALICO=${CALICO//192\.168\.0\.0\/16/${POD_CIDR}} # replace pod cidr
echo "${CALICO}" | kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f -

echo done.
) 2>&1 | tee /var/log/startup.log
`
