# Provider Implementations

The code in this repository is independent of any specific deployment environment.
Provider specific code is being developed in separate repositories, some of which
are also sponsored by SIG Cluster Lifecycle. Check provider's documentation for
updated info about which API version they are supporting.

## Bootstrap
- [Amazon Elastic Kubernetes Service (EKS)](https://github.com/kubernetes-sigs/cluster-api-provider-aws/tree/main/bootstrap/eks)
- [Kubeadm](https://github.com/kubernetes-sigs/cluster-api/tree/main/bootstrap/kubeadm)
- [MicroK8s](https://github.com/canonical/cluster-api-bootstrap-provider-microk8s)
- [Oracle Cloud Native Environment (OCNE)](https://github.com/verrazzano/cluster-api-provider-ocne)
- [Talos](https://github.com/siderolabs/cluster-api-bootstrap-provider-talos)

## Control Plane
- [Kubeadm](https://github.com/kubernetes-sigs/cluster-api/tree/main/controlplane/kubeadm)
- [MicroK8s](https://github.com/canonical/cluster-api-control-plane-provider-microk8s)
- [Nested](https://github.com/kubernetes-sigs/cluster-api-provider-nested)
- [Oracle Cloud Native Environment (OCNE)](https://github.com/verrazzano/cluster-api-provider-ocne)
- [Talos](https://github.com/siderolabs/cluster-api-control-plane-provider-talos)

## Infrastructure
- [AWS](https://cluster-api-aws.sigs.k8s.io/)
- [Azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure)
- [Azure Stack HCI](https://github.com/microsoft/cluster-api-provider-azurestackhci)
- [Bring Your Own Host (BYOH)](https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost)
- [CloudStack](https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack)
- [CoxEdge](https://github.com/coxedge/cluster-api-provider-coxedge)
- [DigitalOcean](https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean)
- [Equinix Metal (formerly Packet)](https://github.com/kubernetes-sigs/cluster-api-provider-packet)
- [Google Cloud Platform (GCP)](https://github.com/kubernetes-sigs/cluster-api-provider-gcp)
- [Hetzner](https://github.com/syself/cluster-api-provider-hetzner)
- [IBM Cloud](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud)
- [KubeKey](https://github.com/kubesphere/kubekey)
- [KubeVirt](https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt)
- [MAAS](https://github.com/spectrocloud/cluster-api-provider-maas)
- [Metal3](https://github.com/metal3-io/cluster-api-provider-metal3)
- [Microvm](https://github.com/weaveworks-liquidmetal/cluster-api-provider-microvm)
- [Nested](https://github.com/kubernetes-sigs/cluster-api-provider-nested)
- [Nutanix](https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix)
- [Oracle Cloud Infrastructure (OCI)](https://github.com/oracle/cluster-api-provider-oci)
- [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack)
- [Outscale](https://github.com/outscale/cluster-api-provider-outscale)
- [Sidero](https://github.com/siderolabs/sidero)
- [Tinkerbell](https://github.com/tinkerbell/cluster-api-provider-tinkerbell)
- [vcluster](https://github.com/loft-sh/cluster-api-provider-vcluster)
- [Virtink](https://github.com/smartxworks/cluster-api-provider-virtink)
- [VMware Cloud Director](https://github.com/vmware/cluster-api-provider-cloud-director)  
- [vSphere](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere)

## IP Address Management (IPAM)
- [In Cluster](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster)

## Addon
- [Helm](https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/)

## API Adopters

Following are the implementations managed by third-parties adopting the standard cluster-api and/or machine-api being developed here.

* [Gardener Machine controller manager](https://github.com/gardener/machine-controller-manager/tree/cluster-api)
* [Kubermatic machine controller](https://github.com/kubermatic/machine-controller)
* [OpenShift Machine API Operator](https://github.com/openshift/machine-api-operator)
