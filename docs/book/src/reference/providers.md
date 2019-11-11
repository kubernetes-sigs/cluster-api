## Provider Implementations

The code in this repository is independent of any specific deployment environment.
Provider specific code is being developed in separate repositories](some of which
are also sponsored by SIG Cluster Lifecycle. Providers marked in bold are known to
support v1alpha2 API types.

## Bootstrap
- [**Kubeadm**](https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm)
- [**Talos**](https://github.com/talos-systems/cluster-api-bootstrap-provider-talos)


## Infrastructure
- [**AWS**](https://github.com/kubernetes-sigs/cluster-api-provider-aws)
- [Azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure)
- [Baidu Cloud](https://github.com/baidu/cluster-api-provider-baiducloud)
- [Bare Metal](https://github.com/metal3-io/cluster-api-provider-baremetal)
- [DigitalOcean](https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean)
- [Exoscale](https://github.com/exoscale/cluster-api-provider-exoscale)
- [GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp)
- [IBM Cloud](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud)
- [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack)
- [Packet](https://github.com/packethost/cluster-api-provider-packet)
- [Tencent Cloud](https://github.com/TencentCloud/cluster-api-provider-tencent)
- [**vSphere**](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere)
- [Alibaba Cloud](https://github.com/oam-oss/cluster-api-provider-alicloud)

## API Adopters

Following are the implementations managed by third-parties adopting the standard cluster-api and/or machine-api being developed here.

  * [Kubermatic machine controller](https://github.com/kubermatic/machine-controller/tree/master)
  * [Machine API Operator](https://github.com/openshift/machine-api-operator/tree/master)
  * [Machine controller manager](https://github.com/gardener/machine-controller-manager/tree/cluster-api)
