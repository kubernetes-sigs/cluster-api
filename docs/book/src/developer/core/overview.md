# Developing "core" Cluster API

This section of the book is about developing "core" Cluster API.

With "core" Cluster API we refer to the common set of API and controllers that are required to run 
any Cluster API provider.

Please note that in the Cluster API code base, side by side of "core" Cluster API components there
is also a limited number of in-tree providers:

- Kubeadm bootstrap provider (CAPBK)
- Kubeadm control plane provider (KCP)
- Docker infrastructure provider (CAPD) - The Docker provider is not designed for production use and is intended for development & test only.
- In Memory infrastructure provider (CAPIM) - The In Memory provider is not designed for production use and is intended for development & test only.

Please refer to [Developing providers](../providers/overview.md) for documentation about in-tree providers (and out of tree providers too).
