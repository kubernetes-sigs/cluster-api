# Upgrading Cluster API and Providers

The [clusterctl upgrade] command can be used to identify possible targets for upgrades.

```bash
clusterctl upgrade plan
```

Produces an output similar to this:

```bash
Checking cert-manager version...
Cert-Manager will be upgraded from "v1.18.2" to "v1.19.1"

Checking new release availability...

Management group: capi-system/cluster-api, latest release available for the v1beta2 API Version of Cluster API (contract):

NAME                    NAMESPACE                           TYPE                     CURRENT VERSION   NEXT VERSION
bootstrap-kubeadm       capi-kubeadm-bootstrap-system       BootstrapProvider        v1.12.0           v1.13.0
control-plane-kubeadm   capi-kubeadm-control-plane-system   ControlPlaneProvider     v1.12.0           v1.13.0
cluster-api             capi-system                         CoreProvider             v1.12.0           v1.13.0
infrastructure-docker   capd-system                         InfrastructureProvider   v1.12.0           v1.13.0
```

The output contains the latest release available for each Cluster API contract version available at the moment.

You can now apply the upgrade by executing the following command:

```bash
clusterctl upgrade apply --contract v1beta2
```

See [clusterctl upgrade] for more details.

If you are not using [clusterctl], the same operation can be performed using [Cluster API operator], Helm, kapp, 
flux, Argo CD, kustomize, or kubectl.

## When to Upgrade

In general, it's recommended to upgrade to the latest version of Cluster API and providers
to take advantage of bug fixes, new features and improvements.

## Pre-flight Checks
 
Before upgrading Cluster API and providers a few pre-flight checks should be performed:

- Ensure that the version of Cluster API is compatible with the Kubernetes version of the management cluster. See [kubernetes versions support].
- Ensure that the version of Cluster API is compatible with the Kubernetes version of the workload clusters. See [kubernetes versions support].
- Check which API versions are supported by Cluster API and ensure that providers are not using a different version. See [API versions support]. 
- Check which Cluster API contract is supported by Cluster API and that providers are implementing it. See [Contract versions support].

If you are upgrading using [clusterctl upgrade], the command will automatically take care of checking contract compatibility
and suggest supported combinations of Cluster API and provider versions.

If moving between different API versions, there may be additional tasks that you need to complete. Detailed
instructions are provided in release notes.

[clusterctl]: ../../../reference/clusterctl/overview.md
[clusterctl upgrade]: ../../../reference/clusterctl/commands/upgrade.md
[Cluster API operator]: https://cluster-api-operator.sigs.k8s.io/
[kubernetes versions support]: ../../../reference/versions.md#kubernetes-versions-support
[API versions support]: ../../../reference/versions.md#cluster-api-release-vs-api-versions
[Contract versions support]: ../../../reference/versions.md#cluster-api-release-vs-contract-versions
