# Cluster API v1alpha3 compared to v1alpha4

## Minimum Go version

- The Go version used by Cluster API is now Go 1.15+

## Controller Runtime version

- The Controller Runtime version is now v0.7.+

## Kind version

- The KIND version used for this release is v0.9.x

## Upgrade kube-rbac-proxy to v0.5.0

- Find and replace the `kube-rbac-proxy` version (usually the image is `gcr.io/kubebuilder/kube-rbac-proxy`) and update it to `v0.5.0`.

## The controllers.DeleteNodeAnnotation constant has been removed

- This annotation `cluster.k8s.io/delete-machine` was originally deprecated a while ago when we moved our types under the `x-k8s.io` domain.

## The controllers.DeleteMachineAnnotation has been moved to v1alpha4.DeleteMachineAnnotation

- This annotation was previously exported as part of the controllers package, instead this should be a versioned annotation under the api packages.

## Align manager flag names with upstream Kubernetes components

- Rename `--metrics-addr` to `--metrics-bind-addr`
- Rename `--leader-election` to `--leader-elect`
