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

- v1beta2 is the hub and storage version. Every field added to or changed in v1beta2 is mirrored identically in
  v1beta1, so conversion needs no custom code ([details](#versions-and-conversion)).
- Follow [kube-api-linter](#kubernetes-api-guidelines): struct fields are values with `omitempty,omitzero`, pointers
  only when the zero value is meaningful, every field is `+optional` or `+required` ([details](#writing-api-types)).
- No kubebuilder defaulting markers; don't silence kube-api-linter with `//nolint` without a reason.
- New alpha fields or behaviour are gated by a feature gate in the same PR ([details](#feature-gates)).
- After changing types run `make generate` and `make test-conversions`; never hand-edit `zz_generated.*` or generated CRDs.
- Don't add tests for conversion code; the fuzz conversion tests already cover it.

## Kubernetes API guidelines 

This project follows the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md).

We enforce the API conventions via [kube-api-linter](https://github.com/kubernetes-sigs/kube-api-linter).
The corresponding configuration field can be found [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/.golangci-kal.yml).

API versioning and guarantees are inspired by the [Kubernetes deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
and [API change guidelines](https://github.com/kubernetes/community/blob/f0eec4d19d407c13681431b3c436be67da8c448d/contributors/devel/sig-architecture/api_changes.md).

## Writing API types

The following applies to the current API version (v1beta2) and is enforced by kube-api-linter (`make lint-api`,
configuration in `.golangci-kal.yml`). v1beta1 and older versions are excluded from kube-api-linter and are not a
model for new code.

- Optional or required struct-typed fields are values, not pointers, with the tag `json:"x,omitempty,omitzero"`.
  Add `// +kubebuilder:validation:MinProperties=1` to the struct type so `{}` isn't a valid value.
- Use a pointer only when the zero value is meaningful and different from "unset", e.g. a `*[]corev1.Taint` with
  `MinItems=0` so an explicit `[]` is preserved.
- Optional scalars get `omitempty` plus a validation that makes the zero value invalid (`MinLength=1`, `Minimum=1`, ...).
- Every field is marked `+optional` or `+required`; required fields are values, not pointers.
- Add `MinItems=1` to lists where an empty list isn't meaningful.
- Don't share a type between fields whose validation differs; split it into separate types with their own markers.
- Don't use kubebuilder defaulting markers. Controller-side defaulting can be described in godoc ("Defaults to X").
- Don't silence kube-api-linter with `//nolint` unless the comment explains why (e.g. "field will be removed when
  v1beta1 is removed"). Prefer a targeted `exclude` (with `path` and `text`) in `.golangci-kal.yml`, with a comment.
- Put a field where it belongs structurally, e.g. directly under `status` instead of nested in an unrelated struct.
- Godoc: field comments start with the JSON name of the field; type comments are one sentence ("X holds/defines ...").
  Keep the godoc of sibling fields (control plane vs. workers, v1beta1 vs. v1beta2) in sync.
- Changing the fields of a type used by KCP may also require updating the allowed paths in the KCP `ValidateUpdate` webhook.

### Versions and conversion

v1beta2 is the hub and storage version, v1beta1 is a served old version.

- Every field added to or changed in v1beta2 is added to v1beta1 as an exact copy (same fields, same pointer/value
  choice, same `omitzero`; use `*bool` in v1beta1 where v1beta2 needs the tri-state), so conversion is handled by the
  generated `autoConvert` functions. If a field can't be mirrored, explain why in a `// Note:` comment.
- The hand-written `Convert_*` functions used by conversion-gen live in `api/*/v1beta1/conversion.go`; the
  `ConvertTo`/`ConvertFrom` implementations live in `<provider>/webhooks/conversion/`. Remove custom conversion code
  and fuzz-test tweaks that become unnecessary once the types match.
- Fields dropped between versions need conversion-preserving code and must pass `TestFuzzyConversion` (`make test-conversions`).

### Feature gates

New alpha behaviour is gated by a feature gate in the same PR:

- A constant with an `// alpha: vX.Y` godoc in `feature/feature.go`, and an entry in the default feature gates map.
- The `EXP_*` variable in the `--feature-gates` flag in `<provider>/config/manager/manager.yaml` and in
  `test/e2e/config/docker.yaml`, and an entry in
  the [feature gates reference](../../reference/feature-gates.md).
- The validation webhook rejects the field when the gate is disabled, using `field.Forbidden(...)` with a message like
  "can only be set if the X feature flag is enabled".

## Other considerations

Following considerations should apply when working to API changes in this project:

- [CRD relations](../../reference/api/crd-relationships.md) allows to model the entire set of objects in a cluster, including also provider's objects
- [Owner references](../../reference/api/owner-references.md) are the foundation of several internal processes
- [Metadata propagation](../../reference/api/metadata-propagation.md) defines how metadata propagates across API kinds
- [Printer columns](../../reference/api/printer-columns.md) guidelines