# Provider Implementations

The code in this repository is independent of any specific deployment environment.
Provider specific code is being developed in separate repositories, some of which
are also sponsored by SIG Cluster Lifecycle. Check provider's documentation for
updated info about which API version they are supporting.

## Bootstrap
- [Kubeadm](https://github.com/kubernetes-sigs/cluster-api/tree/main/bootstrap/kubeadm)
- [Talos](https://github.com/siderolabs/cluster-api-bootstrap-provider-talos)
- [EKS](https://github.com/kubernetes-sigs/cluster-api-provider-aws/tree/main/bootstrap/eks)

## Infrastructure
- [Alibaba Cloud](https://github.com/oam-oss/cluster-api-provider-alicloud)
- [AWS](https://cluster-api-aws.sigs.k8s.io/)
- [Azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure)
- [Azure Stack HCI](https://github.com/microsoft/cluster-api-provider-azurestackhci)
- [Baidu Cloud](https://github.com/baidu/cluster-api-provider-baiducloud)
- [BYOH](https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost)
- [CloudStack](https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack)
- [Metal3](https://github.com/metal3-io/cluster-api-provider-metal3)
- [DigitalOcean](https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean)
- [Exoscale](https://github.com/exoscale/cluster-api-provider-exoscale)
- [GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp)
- [Hetzner](https://github.com/syself/cluster-api-provider-hetzner)
- [IBM Cloud](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud)
- [KubeVirt](https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt)
- [MAAS](https://github.com/spectrocloud/cluster-api-provider-maas)
- [vcluster](https://github.com/loft-sh/cluster-api-provider-vcluster)
- [Nested](https://github.com/kubernetes-sigs/cluster-api-provider-nested)
- [Nutanix](https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix)
- [OCI](https://github.com/oracle/cluster-api-provider-oci)
- [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack)
- [Equinix Metal (formerly Packet)](https://github.com/kubernetes-sigs/cluster-api-provider-packet)
- [Sidero](https://github.com/siderolabs/sidero)
- [Tencent Cloud](https://github.com/TencentCloud/cluster-api-provider-tencent)
- [vSphere](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere)
- [Microvm](https://github.com/weaveworks-liquidmetal/cluster-api-provider-microvm)

## API Adopters

Following are the implementations managed by third-parties adopting the standard cluster-api and/or machine-api being developed here.

* [Kubermatic machine controller](https://github.com/kubermatic/machine-controller)
* [OpenShift Machine API Operator](https://github.com/openshift/machine-api-operator)
* [Gardener Machine controller manager](https://github.com/gardener/machine-controller-manager/tree/cluster-api)
