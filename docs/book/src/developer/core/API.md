# API

In Cluster API, API resources are defined using Kubernetes Custom Resources (CRD).

The API defines the main contract with the Cluster API users. This makes API design a critical part of Cluster API development and usually:

- Breaking/major API changes should go through the CAEP process and be strictly synchronized with the major
  release cadence.
- Non-breaking/minor API changes can go in minor releases; non-breaking changes are generally:
    - additive in nature
    - default to pre-existing behavior
    - optional as part of the API contract

## Kubernetes API guidelines 

This project follows the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md).

We enforce the API conventions via [kube-api-linter](https://github.com/kubernetes-sigs/kube-api-linter).
The corresponding configuration field can be found [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/.golangci-kal.yml).

API versioning and guarantees are inspired by the [Kubernetes deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
and [API change guidelines](https://github.com/kubernetes/community/blob/f0eec4d19d407c13681431b3c436be67da8c448d/contributors/devel/sig-architecture/api_changes.md).

## Other considerations

Following considerations should apply when working to API changes in this project:

- [CRD relations](../../reference/api/crd-relationships.md) allows to model the entire set of objects in a cluster, including also provider's objects
- [Owner references](../../reference/api/owner-references.md) are the foundation of several internal processes
- [Metadata propagation](../../reference/api/metadata-propagation.md) defines how metadata propagates across API kinds
- [Printer columns](../../reference/api/printer-columns.md) guidelines