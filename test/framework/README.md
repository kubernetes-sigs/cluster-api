# CAPI e2e testing framework

This framework aims to define common end-to-end patterns that can be reused across Cluster API providers.

## Usage

Use this framework as you see fit. If there are pieces that don't work for you, do not use them. If there are pieces
that almost work for you, customize them. If this does not fit your e2e testing please file an issue and we can discuss
your use case and find a nice solution for everyone.

### Features

#### Optionally override the images that get loaded onto the management cluster.

This feature allows you to obtain a CAPI management image locally and use that specific image in your kind cluster.
If you do not have one locally then the latest :master image will be used on the management cluster.

### Contents

This framework contains

* A [Go interface][mgmt] to a management cluster
    * A [struct that implements][impl] the management cluster interface using `kind` as the backend.
* A series of behavioral tests

[mgmt]: ./interfaces.go
[impl]: ./management/kind/mgmt.go

## Requirements

### Code

* You must use [ginkgo][ginkgo] for your testing framework.

[ginkgo]: https://onsi.github.io/ginkgo/

## Examples

To see this framework in use please take a look at the [docker provider found in the cluster-api repository][capd].

[capd]: https://github.com/kubernetes-sigs/cluster-api/tree/master/test/infrastructure/docker
