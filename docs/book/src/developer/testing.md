# Testing Cluster API

## Basic tests

### Unit tests

Unit tests run very quickly. They don't require any additional services to execute. They focus on individual pieces of
logic and allow integration bugs to slip through. They are fast and great for getting the initial implementation worked
out.

### `envtest`

`envtest` is a testing environment that is provided by `kubebuilder`. This environment spins up a local instance of
etcd and the kube-apiserver. This allows reconcilers to do all the things they need to do to function very similarly
to how they would in a real environment. These tests provide integration testing between reconcilers and kubernetes.

### Running unit and `envtest` tests

Using the `test` target through `make` will run all of the unit and `envtest` tests.

## Integration tests

Integration tests use a real cluster and real dependencies to run tests. The dependencies are managed manually and are
not meant to be run locally. This is used during CI to ensure basic functionality works as expected.

See `scripts/ci-integration.sh` for more details.

## End-to-end tests

The end-to-end tests are similar to the integration tests except they are designed to manage dependencies for you and 
have a more complete test using a docker provider to test the Cluster API mechanisms. The end-to-end tests can run
locally without modifying the host machine or on a CI platform.

### Running the end-to-end tests

#### Environment variables

The test run can be controlled to some degree by environment variables.

* `SKIP_RESOURCE_CLEANUP`: Will prevent the end-to-end tests from cleaning up the local resources they create. This is
useful when debugging an error that requires cluster inspection. After the error occurs and the tests fail, the cluster
remains. You can then `docker exec` into a running container to get information to assist in debugging. This parameter
is also useful if you simply want a local cluster to experiment with. After the tests run and succeed you are left
with a cluster in a known working state for experimentation.

* `FOCUS`: The `FOCUS` variable allows running of certain tests. The default on CI is `FOCUS='Basic'`. In order to run
all the tests, set `FOCUS='Basic|Full'`. This will run the basic test and then run all other tests.

### Types of end-to-end runs

`make test-capd-e2e-full` will build all manifests and provider images then use those manifests and images in the test
run. This is great if you're modifying API types in any of the providers.

`make test-capd-e2e-images` will only build the images for all providers then use those images during the e2e tests and
use whatever manifests exist on disk. This is good if you're only updating controller code.

`make test-capd-e2e` will only build the docker provider image and use whatever provider images already exist on disk.
This is good if you're working on the test framework itself. You'll likely want to build the images at least once
and then use this for a faster test run.

### Examples

* `make test-capd-e2e-full SKIP_RESOURCE_CLEANUP=true`
* `make test-capd-e2e-images`
* `make test-capd-e2e FOCUS='Basic|Full'`
