# Cluster API Provider Docker (CAPD)

CAPD is a reference implementation of an infrastructure provider for the Cluster API project using Docker.

**NOTE:** The Docker provider is **not** designed for production use and is intended for development environments only.

This is one out of three components needed to run a Cluster API management cluster.

For a complete overview, please refer to the documentation available [here](https://github.com/kubernetes-sigs/cluster-api/tree/main/bootstrap/kubeadm#cluster-api-bootstrap-provider-kubeadm) which uses CAPD as an example infrastructure provider.

## CAPD Goals

* To be a the reference implementation of an infrastructure provider.
* The code is highly trusted and used in testing of ClusterAPI.
* This provider can be used as a guide for developers looking to implement their own infrastructure provider.

## Testing

In order to test your local changes, go to the top level directory of this project, `cluster-api/` and run
`make -C test/infrastructure/docker test` to run the unit tests. 

**Note:** `make test-e2e` runs the CAPI E2E tests that are based on CAPD (CAPD does not have a separated e2e suite).

This make target will build an image based on the local source code and use that image during testing.