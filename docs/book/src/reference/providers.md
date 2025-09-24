# Provider Implementations

The Cluster API project supports ecosystem growth and extensibility.

As part of this effort, everyone is welcome to add to the list below both providers sponsored
by SIG Cluster Lifecycle as well as providers from other open-source repositories.

Each provider is the responsibility of the respective maintainers and we highly recommend
everyone interested in a specific provider to engage with the corresponding team to show support, share use cases,
learn more about the other users of the same provider. 

We also recommend to read provider's documentation carefully, test it, and perform a proper
due diligence before deciding to use a provider in production, like you will do for any other open source project.

We are deeply thankful to all the the providers in the list, because they are a concrete proof
of the liveness of the ecosystem, a relevant part of the history of this community, and a valuable
source of inspiration and ideas for others.

## Bootstrap
- [Amazon Elastic Kubernetes Service (EKS)](https://github.com/kubernetes-sigs/cluster-api-provider-aws/tree/main/bootstrap/eks)
- [Canonical Kubernetes Platform](https://github.com/canonical/cluster-api-k8s)
- [k0smotron/k0s](https://github.com/k0sproject/k0smotron)
- [K3s](https://github.com/cluster-api-provider-k3s/cluster-api-k3s)
- [Kubeadm](https://github.com/kubernetes-sigs/cluster-api/tree/main/bootstrap/kubeadm)
- [MicroK8s](https://github.com/canonical/cluster-api-bootstrap-provider-microk8s)
- [RKE2](https://github.com/rancher/cluster-api-provider-rke2)
- [Talos](https://github.com/siderolabs/cluster-api-bootstrap-provider-talos)

## Control Plane
- [Canonical Kubernetes Platform](https://github.com/canonical/cluster-api-k8s)
- [Hosted Control Plane](https://github.com/teutonet/cluster-api-provider-hosted-control-plane)
- [k0smotron/k0s](https://github.com/k0sproject/k0smotron)
- [K3s](https://github.com/cluster-api-provider-k3s/cluster-api-k3s)
- [Kamaji](https://github.com/clastix/cluster-api-control-plane-provider-kamaji)
- [Kubeadm](https://github.com/kubernetes-sigs/cluster-api/tree/main/controlplane/kubeadm)
- [MicroK8s](https://github.com/canonical/cluster-api-control-plane-provider-microk8s)
- [Nested](https://github.com/kubernetes-sigs/cluster-api-provider-nested)
- [RKE2](https://github.com/rancher/cluster-api-provider-rke2)
- [Talos](https://github.com/siderolabs/cluster-api-control-plane-provider-talos)

## Infrastructure
- [Akamai (Linode)](https://linode.github.io/cluster-api-provider-linode/)
- [AWS](https://cluster-api-aws.sigs.k8s.io/)
- [Azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure)
- [Azure Stack HCI](https://github.com/microsoft/cluster-api-provider-azurestackhci)
- [Bring Your Own Host (BYOH)](https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost)
- [CloudStack](https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack)
- [CoxEdge](https://github.com/coxedge/cluster-api-provider-coxedge)
- [DigitalOcean](https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean)
- [Google Cloud Platform (GCP)](https://cluster-api-gcp.sigs.k8s.io/)
- [Harvester](https://github.com/rancher-sandbox/cluster-api-provider-harvester)
- [Hetzner](https://github.com/syself/cluster-api-provider-hetzner)
- [Hivelocity](https://github.com/hivelocity/cluster-api-provider-hivelocity)
- [Huawei Cloud](https://github.com/HuaweiCloudDeveloper/cluster-api-provider-huawei)
- [IBM Cloud](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud)
- [IONOS Cloud](https://github.com/ionos-cloud/cluster-api-provider-ionoscloud)
- [KubeKey](https://github.com/kubesphere/kubekey)
- [k0smotron RemoteMachine (SSH)](https://github.com/k0sproject/k0smotron)
- [KubeVirt](https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt)
- [MAAS](https://github.com/spectrocloud/cluster-api-provider-maas)
- [Metal3](https://github.com/metal3-io/cluster-api-provider-metal3)
- [Microvm](https://github.com/weaveworks-liquidmetal/cluster-api-provider-microvm)
- [Nested](https://github.com/kubernetes-sigs/cluster-api-provider-nested)
- [Nutanix](https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix)
- [Oracle Cloud Infrastructure (OCI)](https://github.com/oracle/cluster-api-provider-oci)
- [OpenNebula](https://github.com/OpenNebula/cluster-api-provider-opennebula)
- [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack)
- [Outscale](https://github.com/outscale/cluster-api-provider-outscale)
- [Proxmox](https://github.com/ionos-cloud/cluster-api-provider-proxmox)
- [Scaleway](https://github.com/scaleway/cluster-api-provider-scaleway)
- [Sidero](https://github.com/siderolabs/sidero)
- [Tinkerbell](https://github.com/tinkerbell/cluster-api-provider-tinkerbell)
- [vcluster](https://github.com/loft-sh/cluster-api-provider-vcluster)
- [Virtink](https://github.com/smartxworks/cluster-api-provider-virtink)
- [VMware Cloud Director](https://github.com/vmware/cluster-api-provider-cloud-director)
- [vSphere](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere)
- [Vultr](https://github.com/vultr/cluster-api-provider-vultr)

## IP Address Management (IPAM)
- [In Cluster](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster)
- [Nutanix](https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix)
- [Metal3](https://github.com/metal3-io/ip-address-manager)

## Addon
- [Fleet](https://github.com/rancher/cluster-api-addon-provider-fleet/)
- [Helm](https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/)
- [Cdk8s](https://github.com/eitco/cluster-api-addon-provider-cdk8s/)

## Runtime Extensions
- [Nutanix](https://github.com/nutanix-cloud-native/cluster-api-runtime-extensions-nutanix/)

## API Adopters

Following are the implementations managed by third-parties adopting the standard cluster-api and/or machine-api being developed here.

* [Gardener Machine controller manager](https://github.com/gardener/machine-controller-manager/tree/cluster-api)
* [Kubermatic machine controller](https://github.com/kubermatic/machine-controller)
* [OpenShift Machine API Operator](https://github.com/openshift/machine-api-operator)
