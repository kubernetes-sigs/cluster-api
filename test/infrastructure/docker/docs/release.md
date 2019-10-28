# Creating a release

Releases for CAPD are very similar to Cluster API.

The differences:

* Tags take the form of `test/infrastructure/docker/v0.0.0`
* Make commands happen from the top level like this: `make -C test/infrastructure/docker release`

Otherwise the process is the same. Please refer to [the Cluster API release document](../../../../docs/developer/releasing.md).
