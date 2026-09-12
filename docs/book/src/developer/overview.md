# Developer Guide

This section of the book focus on the developer guide for Cluster API.

Before starting, read [Contributing to Cluster API](https://cluster-api.sigs.k8s.io/contributing) carefully.

The [getting started](getting-started.md) guide provides an overview about how to set up your development environment.

Please look at [Developing “core” Cluster API](./core/overview.md) if you are looking to contribute to "core" Cluster API;

Please note that in the Cluster API code base, side by side of "core" Cluster API components there
is also a limited number of in-tree providers:

- Kubeadm bootstrap provider (CAPBK)
- Kubeadm control plane provider (KCP)
- Docker infrastructure provider (CAPD) - The Docker provider is not designed for production use and is intended for development & test only.

Please refer to [Developing providers](./providers/overview.md) for documentation about developing in-tree providers as well
as out of tree providers too.
