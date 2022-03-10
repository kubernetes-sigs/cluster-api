# Provider contract

Cluster API defines a contract which requires providers to implement certain fields and patterns in their CRDs and controllers. This contract is required for providers to work correctly with Cluster API.

Cluster API defines the following contracts:

- [Infrastructure provider contract](./cluster-infrastructure.md)
- [Boostrap provider contract](./bootstrap.md)
- [Control Plane provider contract](../../developer/architecture/controllers/control-plane.md#crd-contracts)
- [Machine provider contract](./machine-infrastructure.md)
- [clusterctl provider contract](../../clusterctl/provider-contract.md#clusterctl-provider-contract)
- [Multi tenancy contract](../../developer/architecture/controllers/multi-tenancy.md#contract)

## API version labels
Providers MUST set `cluster.x-k8s.io/<version>` label on all Custom Resource Definitions related to Cluster API starting with v1alpha3.
The label is a map from an API Version of Cluster API (contract) to your Custom Resource Definition versions.
The value is a underscore-delimited (_) list of versions.
Each value MUST point to an available version in your CRD Spec.

The label allows Cluster API controllers to perform automatic conversions for object references, the controllers will pick the last available version in the list if multiple versions are found.
To apply the label to CRDs it’s possible to use commonLabels in your kustomize.yaml file, usually in config/crd.

In this example we show how to map a particular Cluster API contract version to your own CRD using Kustomize’s commonLabels feature:

```yaml
commonLabels:
cluster.x-k8s.io/v1alpha2: v1alpha1
cluster.x-k8s.io/v1alpha3: v1alpha2
cluster.x-k8s.io/v1beta1:  v1beta1
```

An example of this is in the [Kubeadm Bootstrap provider](https://github.com/kubernetes-sigs/cluster-api/blob/release-1.1/controlplane/kubeadm/config/crd/kustomization.yaml).

## Improving and contributing to the contract

The definition of the contract between Cluster API and providers may be changed in future versions of Cluster API. The Cluster API maintainers welcome feedback and contributions to the contract in order to improve how it's defined, its clarity and visibility to provider implementers and its suitability across the different kinds of Cluster API providers. To provide feedback or open a discussion about the provider contract please [open an issue on the Cluster API](https://github.com/kubernetes-sigs/cluster-api/issues/new?assignees=&labels=&template=feature_request.md) repo or add an item to the agenda in the [Cluster API community meeting](http://git.k8s.io/community/sig-cluster-lifecycle/README.md#cluster-api).