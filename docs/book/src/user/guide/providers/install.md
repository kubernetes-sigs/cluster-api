# Installing Cluster API and Providers

The simplest option to install Cluster API providers is to use [clusterctl], a utility that
offers the [clusterctl init] command that was specifically designed for this task. For example:

```bash
clusterctl init --infrastructure vsphere
```

This initializes a Cluster API management cluster with Cluster API itself, the kubeadm bootstrap and control plane provider,
and the vSphere provider. See [clusterctl init] for more details.

> [!IMPORTANT]
> Depending on the infrastructure provider you are planning to use 
> some additional prerequisites should be satisfied before getting started with Cluster API. 
> See [Initialize the management cluster] for more details.

> [!IMPORTANT]
> See [Feature Gates] for enabling additional features.

If you don't want to use [clusterctl], the [Cluster API operator] project instead offers a GitOps-friendly option.

Alternatives include managing manifests for Cluster API and providers with tools like Helm, kapp, flux, Argo CD, kustomize,
or kubectl.

[clusterctl]: ../../../reference/clusterctl/overview.md
[clusterctl init]: ../../../reference/clusterctl/commands/init.md
[Initialize the management cluster]: ../../quick-start.md#initialize-the-management-cluster
[Feature Gates]: ../../../reference/feature-gates.md
[Cluster API operator]: https://cluster-api-operator.sigs.k8s.io/

