# Repository layout

Different providers may differ in their directory layout. Beginning with
the move to using CRDs and `kubebuilder` the general layout follows that
defined by the `kubebuilder` project.

## Go package Structure

##### cmd/...

The `cmd` package contains the manager main program.  Manager is responsible 
for initializing shared dependencies and starting / stopping Controllers. 

The `cmd` package also contains the shared `clusterctl`.

##### pkg/apis/...

The `pkg/apis/...` packages contains the API resource definitions. Users edit 
the `*_types.go` files under this directory to implement their API definitions.

Each resource lives in a `pkg/apis/<api-group-name>/<api-version-name>/<api-kind-name>_types.go` file.

##### pkg/controller

The `controller` package instantiates Cluster API controllers with provider specific Actuators.

## Additional directories and files

In addition to the packages above, a Kubebuilder project has several other directories and files.

##### Makefile

A Makefile is created with targets to build, test, run and deploy the controller artifacts
for development as well as production workflows

##### Dockerfile

A Dockerfile is scaffolded to build a container image for your Manager.

##### config/...

Kubebuilder creates yaml config for installing the CRDs and related objects under config/.

- config/crds
- config/rbac
- config/manager
- config/samples

##### docs/...

API reference documentation, user defined API samples and API conceptual documentation go here.

