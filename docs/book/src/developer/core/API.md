# API

In Cluster API, API resources are defined using Kubernetes Custom Resources (CRD).

The API defines the main contract with the Cluster API users. This makes API design a critical part of Cluster API development and usually:

- Breaking/major API changes should go through the CAEP process and be strictly synchronized with the major
  release cadence.
- Non-breaking/minor API changes can go in minor releases; non-breaking changes are generally:
    - additive in nature
    - default to pre-existing behavior
    - optional as part of the API contract

## Rules

- Breaking changes within the same version of the API are not allowed.
- A change to the API must be discussed with the community (e.g. in an issue or a CAEP) and approved by maintainers first.
- Follow the [Kubernetes API guidelines](#kubernetes-api-guidelines) and comply with the kube-api-linter rules
  (`make lint-api`).
- Don't silence kube-api-linter with `//nolint` unless the comment explains why (e.g. "field will be removed when
  v1beta1 is removed"). Prefer a targeted `exclude` (with `path` and `text`) in `.golangci-kal.yml`, with a comment over an inline //nolint comment.
- v1beta2 is the hub and storage version. Every field added to or changed in v1beta2 should be mirrored identically in
  v1beta1, so conversion needs no custom code. If a field can't be mirrored, explain why in a `// Note:` comment.
- Don't add tests for conversion code; the fuzz conversion tests already cover it.
- Don't use kubebuilder defaulting markers. Controller-side defaulting can be described in godoc ("Defaults to X").
- Don't share a type between fields whose validation differs; split it into separate types with their own markers.
- Put a field where it belongs structurally, e.g. directly under `status` instead of nested in an unrelated struct.
- Godoc: field comments start with the JSON name of the field; type comments are one sentence ("X holds/defines ...").
  Keep the godoc of sibling fields (control plane vs. workers, v1beta1 vs. v1beta2) in sync.
- After changing API types run `make generate` and `make test-conversions`; never hand-edit `zz_generated.*` or generated CRDs.
- API changes must also be listed in the "API Changes" section of the [migration doc](../providers/migrations/overview.md)
  for the next release.

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
