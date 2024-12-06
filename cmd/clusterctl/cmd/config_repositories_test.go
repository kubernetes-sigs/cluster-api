/*
Copyright 2020 The Kubernetes Authors.

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

package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_runGetRepositories(t *testing.T) {
	t.Run("prints output", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir, err := os.MkdirTemp("", "cc")
		g.Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		path := filepath.Join(tmpDir, "clusterctl.yaml")
		g.Expect(os.WriteFile(path, []byte(template), 0600)).To(Succeed())

		buf := bytes.NewBufferString("")

		for _, val := range RepositoriesOutputs {
			cro.output = val
			g.Expect(runGetRepositories(path, buf)).To(Succeed())
			out, err := io.ReadAll(buf)
			g.Expect(err).ToNot(HaveOccurred())

			// Use gomega's BeComparableTo as opposed to Equals to compare output which uses gocmp under
			// the hood and correctly prints any differences between the two strings.
			if val == RepositoriesOutputText {
				g.Expect(string(out)).To(BeComparableTo(expectedOutputText))
			} else if val == RepositoriesOutputYaml {
				g.Expect(string(out)).To(BeComparableTo(expectedOutputYaml))
			}
		}
	})

	t.Run("returns error for bad cfgFile path", func(t *testing.T) {
		g := NewWithT(t)
		buf := bytes.NewBufferString("")
		g.Expect(runGetRepositories("do-not-exist", buf)).ToNot(Succeed())
	})

	t.Run("returns error for nil writer", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(runGetRepositories("do-exist", nil)).ToNot(Succeed())
	})

	t.Run("returns error for bad template", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir, err := os.MkdirTemp("", "cc")
		g.Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		path := filepath.Join(tmpDir, "clusterctl.yaml")
		g.Expect(os.WriteFile(path, []byte("providers: foobar"), 0600)).To(Succeed())

		buf := bytes.NewBufferString("")
		g.Expect(runGetRepositories(path, buf)).ToNot(Succeed())
	})
}

var template = `---
providers:
  # add a custom provider
  - name: "my-infra-provider"
    url: "/home/.config/cluster-api/overrides/infrastructure-docker/latest/infrastructure-components.yaml"
    type: "InfrastructureProvider"
  # add a custom provider
  - name: "another-provider"
    url: "./bootstrap-components.yaml"
    type: "BootstrapProvider"
  # bad url
  - name: "aws"
    url: "my-aws-infrastructure-components.yaml"
    type: "InfrastructureProvider"
  # override a pre-defined provider
  - name: "cluster-api"
    url: "https://github.com/myorg/myforkofclusterapi/releases/latest/core_components.yaml"
    type: "CoreProvider"
`

var expectedOutputText = `NAME                    TYPE                       URL                                                                                               FILE
cluster-api             CoreProvider               https://github.com/myorg/myforkofclusterapi/releases/latest/                                      core_components.yaml
another-provider        BootstrapProvider          ./                                                                                                bootstrap-components.yaml
canonical-kubernetes    BootstrapProvider          https://github.com/canonical/cluster-api-k8s/releases/latest/                                     bootstrap-components.yaml
k0sproject-k0smotron    BootstrapProvider          https://github.com/k0sproject/k0smotron/releases/latest/                                          bootstrap-components.yaml
kubeadm                 BootstrapProvider          https://github.com/kubernetes-sigs/cluster-api/releases/latest/                                   bootstrap-components.yaml
kubekey-k3s             BootstrapProvider          https://github.com/kubesphere/kubekey/releases/latest/                                            bootstrap-components.yaml
microk8s                BootstrapProvider          https://github.com/canonical/cluster-api-bootstrap-provider-microk8s/releases/latest/             bootstrap-components.yaml
ocne                    BootstrapProvider          https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/                          bootstrap-components.yaml
rke2                    BootstrapProvider          https://github.com/rancher/cluster-api-provider-rke2/releases/latest/                             bootstrap-components.yaml
talos                   BootstrapProvider          https://github.com/siderolabs/cluster-api-bootstrap-provider-talos/releases/latest/               bootstrap-components.yaml
canonical-kubernetes    ControlPlaneProvider       https://github.com/canonical/cluster-api-k8s/releases/latest/                                     control-plane-components.yaml
k0sproject-k0smotron    ControlPlaneProvider       https://github.com/k0sproject/k0smotron/releases/latest/                                          control-plane-components.yaml
kamaji                  ControlPlaneProvider       https://github.com/clastix/cluster-api-control-plane-provider-kamaji/releases/latest/             control-plane-components.yaml
kubeadm                 ControlPlaneProvider       https://github.com/kubernetes-sigs/cluster-api/releases/latest/                                   control-plane-components.yaml
kubekey-k3s             ControlPlaneProvider       https://github.com/kubesphere/kubekey/releases/latest/                                            control-plane-components.yaml
microk8s                ControlPlaneProvider       https://github.com/canonical/cluster-api-control-plane-provider-microk8s/releases/latest/         control-plane-components.yaml
nested                  ControlPlaneProvider       https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/                   control-plane-components.yaml
ocne                    ControlPlaneProvider       https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/                          control-plane-components.yaml
rke2                    ControlPlaneProvider       https://github.com/rancher/cluster-api-provider-rke2/releases/latest/                             control-plane-components.yaml
talos                   ControlPlaneProvider       https://github.com/siderolabs/cluster-api-control-plane-provider-talos/releases/latest/           control-plane-components.yaml
aws                     InfrastructureProvider                                                                                                       my-aws-infrastructure-components.yaml
azure                   InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-azure/releases/latest/                    infrastructure-components.yaml
byoh                    InfrastructureProvider     https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost/releases/latest/            infrastructure-components.yaml
cloudstack              InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack/releases/latest/               infrastructure-components.yaml
coxedge                 InfrastructureProvider     https://github.com/coxedge/cluster-api-provider-coxedge/releases/latest/                          infrastructure-components.yaml
digitalocean            InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean/releases/latest/             infrastructure-components.yaml
docker                  InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api/releases/latest/                                   infrastructure-components-development.yaml
gcp                     InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-gcp/releases/latest/                      infrastructure-components.yaml
harvester-harvester     InfrastructureProvider     https://github.com/rancher-sandbox/cluster-api-provider-harvester/releases/latest/                infrastructure-components.yaml
hetzner                 InfrastructureProvider     https://github.com/syself/cluster-api-provider-hetzner/releases/latest/                           infrastructure-components.yaml
hivelocity-hivelocity   InfrastructureProvider     https://github.com/hivelocity/cluster-api-provider-hivelocity/releases/latest/                    infrastructure-components.yaml
ibmcloud                InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/releases/latest/                 infrastructure-components.yaml
in-memory               InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api/releases/latest/                                   infrastructure-components-in-memory-development.yaml
ionoscloud-ionoscloud   InfrastructureProvider     https://github.com/ionos-cloud/cluster-api-provider-ionoscloud/releases/latest/                   infrastructure-components.yaml
k0sproject-k0smotron    InfrastructureProvider     https://github.com/k0sproject/k0smotron/releases/latest/                                          infrastructure-components.yaml
kubekey                 InfrastructureProvider     https://github.com/kubesphere/kubekey/releases/latest/                                            infrastructure-components.yaml
kubevirt                InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt/releases/latest/                 infrastructure-components.yaml
linode-linode           InfrastructureProvider     https://github.com/linode/cluster-api-provider-linode/releases/latest/                            infrastructure-components.yaml
maas                    InfrastructureProvider     https://github.com/spectrocloud/cluster-api-provider-maas/releases/latest/                        infrastructure-components.yaml
metal3                  InfrastructureProvider     https://github.com/metal3-io/cluster-api-provider-metal3/releases/latest/                         infrastructure-components.yaml
my-infra-provider       InfrastructureProvider     /home/.config/cluster-api/overrides/infrastructure-docker/latest/                                 infrastructure-components.yaml
nested                  InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/                   infrastructure-components.yaml
nutanix                 InfrastructureProvider     https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix/releases/latest/             infrastructure-components.yaml
oci                     InfrastructureProvider     https://github.com/oracle/cluster-api-provider-oci/releases/latest/                               infrastructure-components.yaml
openstack               InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases/latest/                infrastructure-components.yaml
outscale                InfrastructureProvider     https://github.com/outscale/cluster-api-provider-outscale/releases/latest/                        infrastructure-components.yaml
packet                  InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases/latest/                   infrastructure-components.yaml
proxmox                 InfrastructureProvider     https://github.com/ionos-cloud/cluster-api-provider-proxmox/releases/latest/                      infrastructure-components.yaml
sidero                  InfrastructureProvider     https://github.com/siderolabs/sidero/releases/latest/                                             infrastructure-components.yaml
tinkerbell-tinkerbell   InfrastructureProvider     https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases/latest/                    infrastructure-components.yaml
vcd                     InfrastructureProvider     https://github.com/vmware/cluster-api-provider-cloud-director/releases/latest/                    infrastructure-components.yaml
vcluster                InfrastructureProvider     https://github.com/loft-sh/cluster-api-provider-vcluster/releases/latest/                         infrastructure-components.yaml
virtink                 InfrastructureProvider     https://github.com/smartxworks/cluster-api-provider-virtink/releases/latest/                      infrastructure-components.yaml
vsphere                 InfrastructureProvider     https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/                  infrastructure-components.yaml
vultr-vultr             InfrastructureProvider     https://github.com/vultr/cluster-api-provider-vultr/releases/latest/                              infrastructure-components.yaml
in-cluster              IPAMProvider               https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases/latest/          ipam-components.yaml
nutanix                 IPAMProvider               https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/releases/latest/        ipam-components.yaml
nutanix                 RuntimeExtensionProvider   https://github.com/nutanix-cloud-native/cluster-api-runtime-extensions-nutanix/releases/latest/   runtime-extensions-components.yaml
helm                    AddonProvider              https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/releases/latest/               addon-components.yaml
`

var expectedOutputYaml = `- File: core_components.yaml
  Name: cluster-api
  ProviderType: CoreProvider
  URL: https://github.com/myorg/myforkofclusterapi/releases/latest/
- File: bootstrap-components.yaml
  Name: another-provider
  ProviderType: BootstrapProvider
  URL: ./
- File: bootstrap-components.yaml
  Name: canonical-kubernetes
  ProviderType: BootstrapProvider
  URL: https://github.com/canonical/cluster-api-k8s/releases/latest/
- File: bootstrap-components.yaml
  Name: k0sproject-k0smotron
  ProviderType: BootstrapProvider
  URL: https://github.com/k0sproject/k0smotron/releases/latest/
- File: bootstrap-components.yaml
  Name: kubeadm
  ProviderType: BootstrapProvider
  URL: https://github.com/kubernetes-sigs/cluster-api/releases/latest/
- File: bootstrap-components.yaml
  Name: kubekey-k3s
  ProviderType: BootstrapProvider
  URL: https://github.com/kubesphere/kubekey/releases/latest/
- File: bootstrap-components.yaml
  Name: microk8s
  ProviderType: BootstrapProvider
  URL: https://github.com/canonical/cluster-api-bootstrap-provider-microk8s/releases/latest/
- File: bootstrap-components.yaml
  Name: ocne
  ProviderType: BootstrapProvider
  URL: https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/
- File: bootstrap-components.yaml
  Name: rke2
  ProviderType: BootstrapProvider
  URL: https://github.com/rancher/cluster-api-provider-rke2/releases/latest/
- File: bootstrap-components.yaml
  Name: talos
  ProviderType: BootstrapProvider
  URL: https://github.com/siderolabs/cluster-api-bootstrap-provider-talos/releases/latest/
- File: control-plane-components.yaml
  Name: canonical-kubernetes
  ProviderType: ControlPlaneProvider
  URL: https://github.com/canonical/cluster-api-k8s/releases/latest/
- File: control-plane-components.yaml
  Name: k0sproject-k0smotron
  ProviderType: ControlPlaneProvider
  URL: https://github.com/k0sproject/k0smotron/releases/latest/
- File: control-plane-components.yaml
  Name: kamaji
  ProviderType: ControlPlaneProvider
  URL: https://github.com/clastix/cluster-api-control-plane-provider-kamaji/releases/latest/
- File: control-plane-components.yaml
  Name: kubeadm
  ProviderType: ControlPlaneProvider
  URL: https://github.com/kubernetes-sigs/cluster-api/releases/latest/
- File: control-plane-components.yaml
  Name: kubekey-k3s
  ProviderType: ControlPlaneProvider
  URL: https://github.com/kubesphere/kubekey/releases/latest/
- File: control-plane-components.yaml
  Name: microk8s
  ProviderType: ControlPlaneProvider
  URL: https://github.com/canonical/cluster-api-control-plane-provider-microk8s/releases/latest/
- File: control-plane-components.yaml
  Name: nested
  ProviderType: ControlPlaneProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/
- File: control-plane-components.yaml
  Name: ocne
  ProviderType: ControlPlaneProvider
  URL: https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/
- File: control-plane-components.yaml
  Name: rke2
  ProviderType: ControlPlaneProvider
  URL: https://github.com/rancher/cluster-api-provider-rke2/releases/latest/
- File: control-plane-components.yaml
  Name: talos
  ProviderType: ControlPlaneProvider
  URL: https://github.com/siderolabs/cluster-api-control-plane-provider-talos/releases/latest/
- File: my-aws-infrastructure-components.yaml
  Name: aws
  ProviderType: InfrastructureProvider
  URL: ""
- File: infrastructure-components.yaml
  Name: azure
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-azure/releases/latest/
- File: infrastructure-components.yaml
  Name: byoh
  ProviderType: InfrastructureProvider
  URL: https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost/releases/latest/
- File: infrastructure-components.yaml
  Name: cloudstack
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack/releases/latest/
- File: infrastructure-components.yaml
  Name: coxedge
  ProviderType: InfrastructureProvider
  URL: https://github.com/coxedge/cluster-api-provider-coxedge/releases/latest/
- File: infrastructure-components.yaml
  Name: digitalocean
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean/releases/latest/
- File: infrastructure-components-development.yaml
  Name: docker
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api/releases/latest/
- File: infrastructure-components.yaml
  Name: gcp
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-gcp/releases/latest/
- File: infrastructure-components.yaml
  Name: harvester-harvester
  ProviderType: InfrastructureProvider
  URL: https://github.com/rancher-sandbox/cluster-api-provider-harvester/releases/latest/
- File: infrastructure-components.yaml
  Name: hetzner
  ProviderType: InfrastructureProvider
  URL: https://github.com/syself/cluster-api-provider-hetzner/releases/latest/
- File: infrastructure-components.yaml
  Name: hivelocity-hivelocity
  ProviderType: InfrastructureProvider
  URL: https://github.com/hivelocity/cluster-api-provider-hivelocity/releases/latest/
- File: infrastructure-components.yaml
  Name: ibmcloud
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/releases/latest/
- File: infrastructure-components-in-memory-development.yaml
  Name: in-memory
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api/releases/latest/
- File: infrastructure-components.yaml
  Name: ionoscloud-ionoscloud
  ProviderType: InfrastructureProvider
  URL: https://github.com/ionos-cloud/cluster-api-provider-ionoscloud/releases/latest/
- File: infrastructure-components.yaml
  Name: k0sproject-k0smotron
  ProviderType: InfrastructureProvider
  URL: https://github.com/k0sproject/k0smotron/releases/latest/
- File: infrastructure-components.yaml
  Name: kubekey
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubesphere/kubekey/releases/latest/
- File: infrastructure-components.yaml
  Name: kubevirt
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt/releases/latest/
- File: infrastructure-components.yaml
  Name: linode-linode
  ProviderType: InfrastructureProvider
  URL: https://github.com/linode/cluster-api-provider-linode/releases/latest/
- File: infrastructure-components.yaml
  Name: maas
  ProviderType: InfrastructureProvider
  URL: https://github.com/spectrocloud/cluster-api-provider-maas/releases/latest/
- File: infrastructure-components.yaml
  Name: metal3
  ProviderType: InfrastructureProvider
  URL: https://github.com/metal3-io/cluster-api-provider-metal3/releases/latest/
- File: infrastructure-components.yaml
  Name: my-infra-provider
  ProviderType: InfrastructureProvider
  URL: /home/.config/cluster-api/overrides/infrastructure-docker/latest/
- File: infrastructure-components.yaml
  Name: nested
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/
- File: infrastructure-components.yaml
  Name: nutanix
  ProviderType: InfrastructureProvider
  URL: https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix/releases/latest/
- File: infrastructure-components.yaml
  Name: oci
  ProviderType: InfrastructureProvider
  URL: https://github.com/oracle/cluster-api-provider-oci/releases/latest/
- File: infrastructure-components.yaml
  Name: openstack
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases/latest/
- File: infrastructure-components.yaml
  Name: outscale
  ProviderType: InfrastructureProvider
  URL: https://github.com/outscale/cluster-api-provider-outscale/releases/latest/
- File: infrastructure-components.yaml
  Name: packet
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases/latest/
- File: infrastructure-components.yaml
  Name: proxmox
  ProviderType: InfrastructureProvider
  URL: https://github.com/ionos-cloud/cluster-api-provider-proxmox/releases/latest/
- File: infrastructure-components.yaml
  Name: sidero
  ProviderType: InfrastructureProvider
  URL: https://github.com/siderolabs/sidero/releases/latest/
- File: infrastructure-components.yaml
  Name: tinkerbell-tinkerbell
  ProviderType: InfrastructureProvider
  URL: https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases/latest/
- File: infrastructure-components.yaml
  Name: vcd
  ProviderType: InfrastructureProvider
  URL: https://github.com/vmware/cluster-api-provider-cloud-director/releases/latest/
- File: infrastructure-components.yaml
  Name: vcluster
  ProviderType: InfrastructureProvider
  URL: https://github.com/loft-sh/cluster-api-provider-vcluster/releases/latest/
- File: infrastructure-components.yaml
  Name: virtink
  ProviderType: InfrastructureProvider
  URL: https://github.com/smartxworks/cluster-api-provider-virtink/releases/latest/
- File: infrastructure-components.yaml
  Name: vsphere
  ProviderType: InfrastructureProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/
- File: infrastructure-components.yaml
  Name: vultr-vultr
  ProviderType: InfrastructureProvider
  URL: https://github.com/vultr/cluster-api-provider-vultr/releases/latest/
- File: ipam-components.yaml
  Name: in-cluster
  ProviderType: IPAMProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases/latest/
- File: ipam-components.yaml
  Name: nutanix
  ProviderType: IPAMProvider
  URL: https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/releases/latest/
- File: runtime-extensions-components.yaml
  Name: nutanix
  ProviderType: RuntimeExtensionProvider
  URL: https://github.com/nutanix-cloud-native/cluster-api-runtime-extensions-nutanix/releases/latest/
- File: addon-components.yaml
  Name: helm
  ProviderType: AddonProvider
  URL: https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/releases/latest/
`
