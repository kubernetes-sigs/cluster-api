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
	"bytes"
	"fmt"
	"text/template"

	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
)

type templateParams struct {
	Token   string
	Cluster *clusterv1.Cluster
	Machine *clusterv1.Machine
}

func nodeMetadata(params templateParams) (map[string]string, error) {
	metadata := map[string]string{}
	var buf bytes.Buffer

	if err := nodeStartupScriptTemplate.Execute(&buf, params); err != nil {
		return nil, err
	}
	metadata["startup-script"] = buf.String()

	return metadata, nil
}

func masterMetadata(params templateParams) (map[string]string, error) {
	metadata := map[string]string{}
	var buf bytes.Buffer

	if err := masterStartupScriptTemplate.Execute(&buf, params); err != nil {
		return nil, err
	}
	metadata["startup-script"] = buf.String()
	return metadata, nil
}

var (
	nodeStartupScriptTemplate   *template.Template
	masterStartupScriptTemplate *template.Template
)

func init() {
	endpoint := func(apiEndpoint *clusterv1.APIEndpoint) string {
		return fmt.Sprintf("%s:%d", apiEndpoint.Host, apiEndpoint.Port)
	}
	funcMap := map[string]interface{}{
		"endpoint": endpoint,
	}
	nodeStartupScriptTemplate = template.Must(template.New("nodeStartupScript").Funcs(funcMap).Parse(nodeStartupScript))
	masterStartupScriptTemplate = template.Must(template.New("masterStartupScript").Parse(masterStartupScript))
}

const nodeStartupScript = `#!/bin/bash

set -e
set -x

(
TOKEN={{ .Token }}
MASTER={{ index .Cluster.Status.APIEndpoints 0 | endpoint }}
MACHINE={{ .Machine.ObjectMeta.Name }}
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.DNSDomain }}
SERVICE_CIDR={{ .Cluster.Spec.ClusterNetwork.ServiceSubnet }}

# Our Debian packages have versions like "1.8.0-00" or "1.8.0-01". Do a prefix
# search based on our SemVer to find the right (newest) package version.
function getversion() {
	name=$1
	prefix=$2
	version=$(apt-cache madison $name | awk '{ print $3 }' | grep ^$prefix | head -n1)
	if [[ -z "$version" ]]; then
		echo Can\'t find package $name with prefix $prefix
		exit 1
	fi
	echo $version
}

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

KUBELET=$(getversion kubelet ${KUBELET_VERSION}-)
KUBEADM=$(getversion kubeadm ${KUBELET_VERSION}-)
KUBECTL=$(getversion kubectl ${KUBELET_VERSION}-)
apt-get install -y kubelet=${KUBELET} kubeadm=${KUBEADM} kubectl=${KUBECTL}

sysctl net.bridge.bridge-nf-call-iptables=1

# kubeadm uses 10th IP as DNS server
CLUSTER_DNS_SERVER=$(prips ${SERVICE_CIDR} | head -n 11 | tail -n 1)

sed -i "s/KUBELET_DNS_ARGS=[^\"]*/KUBELET_DNS_ARGS=--cluster-dns=${CLUSTER_DNS_SERVER} --cluster-domain=${CLUSTER_DNS_DOMAIN}/" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
systemctl daemon-reload
systemctl restart kubelet.service

kubeadm join --token "${TOKEN}" "${MASTER}" --skip-preflight-checks

for tries in $(seq 1 60); do
	kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate --overwrite node $(hostname) machine=${MACHINE} && break
	sleep 1
done

echo done.
) 2>&1 | tee /var/log/startup.log
`

// TODO: actually init the cluster, templatize token, etc.
const masterStartupScript = `#!/bin/bash

set -e
set -x

(
# Our Debian packages have versions like "1.8.0-00" or "1.8.0-01". Do a prefix
# search based on our SemVer to find the right (newest) package version.
function getversion() {
	name=$1
	prefix=$2
	version=$(apt-cache madison $name | awk '{ print $3 }' | grep ^$prefix | head -n1)
	if [[ -z "$version" ]]; then
		echo Can\'t find package $name with prefix $prefix
		exit 1
	fi
	echo $version
}

TOKEN={{ .Token }}
PORT=443
MACHINE={{ .Machine.ObjectMeta.Name }}
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
CONTROL_PLANE_VERSION={{ .Machine.Spec.Versions.ControlPlane }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.DNSDomain }}
POD_CIDR={{ .Cluster.Spec.ClusterNetwork.PodSubnet }}
SERVICE_CIDR={{ .Cluster.Spec.ClusterNetwork.ServiceSubnet }}

curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
touch /etc/apt/sources.list.d/kubernetes.list
sh -c 'echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list'

apt-get update -y

KUBELET=$(getversion kubelet ${KUBELET_VERSION}-)
KUBEADM=$(getversion kubeadm ${KUBELET_VERSION}-)

apt-get install -y \
    socat \
    ebtables \
    docker.io \
    apt-transport-https \
    kubelet=${KUBELET} \
    kubeadm=${KUBEADM} \
    cloud-utils \
    prips

# kubeadm uses 10th IP as DNS server
CLUSTER_DNS_SERVER=$(prips ${SERVICE_CIDR} | head -n 11 | tail -n 1)

systemctl enable docker
systemctl start docker
sed -i "s/KUBELET_DNS_ARGS=[^\"]*/KUBELET_DNS_ARGS=--cluster-dns=${CLUSTER_DNS_SERVER} --cluster-domain=${CLUSTER_DNS_DOMAIN}/" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
systemctl daemon-reload
systemctl restart kubelet.service
` +
	"PRIVATEIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/ip\"`" + `
echo $PRIVATEIP > /tmp/.ip
` +
	"PUBLICIP=`curl --retry 5 -sfH \"Metadata-Flavor: Google\" \"http://metadata/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip\"`" + `

export VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
export ARCH=amd64
curl -sSL https://dl.k8s.io/release/${VERSION}/bin/linux/${ARCH}/kubeadm > /usr/bin/kubeadm
chmod a+rx /usr/bin/kubeadm

kubeadm reset
kubeadm init --apiserver-bind-port ${PORT} --token ${TOKEN} --kubernetes-version v${CONTROL_PLANE_VERSION} \
             --apiserver-advertise-address ${PUBLICIP} --apiserver-cert-extra-sans ${PUBLICIP} ${PRIVATEIP} \
             --service-cidr ${SERVICE_CIDR} 

for tries in $(seq 1 60); do
	kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate --overwrite node $(hostname) machine=${MACHINE} && break
	sleep 1
done

# install wavenet
sysctl net.bridge.bridge-nf-call-iptables=1
export kubever=$(kubectl version --kubeconfig /etc/kubernetes/admin.conf | base64 | tr -d '\n')
kubectl apply --kubeconfig /etc/kubernetes/admin.conf -f "https://cloud.weave.works/k8s/net?k8s-version=$kubever"

echo done.
) 2>&1 | tee /var/log/startup.log
`
