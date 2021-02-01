# Cluster API v1alpha3 compared to v1alpha4

## Minimum Go version

- The Go version used by Cluster API is now Go 1.15+

## Controller Runtime version

- The Controller Runtime version is now v0.7.+

## Kind version

- The KIND version used for this release is v0.9.x

## Upgrade kube-rbac-proxy to v0.8.0

- Find and replace the `kube-rbac-proxy` version (usually the image is `gcr.io/kubebuilder/kube-rbac-proxy`) and update it to `v0.8.0`.

## The controllers.DeleteNodeAnnotation constant has been removed

- This annotation `cluster.k8s.io/delete-machine` was originally deprecated a while ago when we moved our types under the `x-k8s.io` domain.

## The controllers.DeleteMachineAnnotation has been moved to v1alpha4.DeleteMachineAnnotation

- This annotation was previously exported as part of the controllers package, instead this should be a versioned annotation under the api packages.

## Align manager flag names with upstream Kubernetes components

- Rename `--metrics-addr` to `--metrics-bind-addr`
- Rename `--leader-election` to `--leader-elect`

## util.ManagerDelegatingClientFunc has been removed

This function was originally used to generate a delegating client when creating a new manager.

Controller Runtime v0.7.x now uses a `ClientBuilder` in its Options struct and it uses
the delegating client by default under the hood, so this can be now removed.

## Use to Controller Runtime's new fake client builder

- The functions `fake.NewFakeClientWithScheme` and `fake.NewFakeClient` have been deprecated.
- Switch to `fake.NewClientBuilder().WithObjects().Build()` instead, which provides a cleaner interface
  to create a new fake client with objects, lists, or a scheme.

## Multi tenancy

Up until v1alpha3, the need of supporting multiple credentials was addressed by running multiple
instances of the same provider, each one with its own set of credentials while watching different namespaces.

Starting from v1alpha4 instead we are going require that an infrastructure provider should manage different credentials,
each one of them corresponding to an infrastructure tenant.

see [Multi-tenancy](../architecture/controllers/multi-tenancy.md) and [Support multiple instances](../architecture/controllers/support-multiple-instances.md) for
more details.

Specific changes related to this topic will be detailed in this document.

## Change types with arrays of pointers to custom objects

The conversion-gen code from the `1.20.x` release onward generates incorrect conversion functions for types having arrays of pointers to custom objects. Change the existing types to contain objects instead of pointer references.
