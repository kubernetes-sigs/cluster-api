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

* `GINKGO_FOCUS`: The `GINKGO_FOCUS` variable allows running of certain tests. By default, all tests are run if `GINKGO_FOCUS` is not provided. To run
a particular test, set focus to a regex string: e.g., `GINKGO_FOCUS='KCP upgrade'`.

* `USE_EXISTING_CLUSTER`: The `USE_EXISTING_CLUSTER` variable allows using an existing management cluster with providers already installed.
For instance, after running e2e tests once with `SKIP_RESOURCE_CLEANUP=true`, the created management cluster can be used for the following e2e test runs by setting up `KUBECONFIG` variable.

### Running tests

`make docker-build-e2e` will build the images for all providers that will be needed for the e2e test.

`make test-e2e` will run e2e tests by using whatever provider images already exist on disk.
After running `make docker-build-e2e` at least once, this can be used for a faster test run if there are no provider code changes.

**Examples**

* `make docker-build-e2e`

* `make test-e2e GINKGO_FOCUS='upgrade' SKIP_RESOURCE_CLEANUP=true USE_EXISTING_CLUSTER=false`

See [e2e development] for more information on developing e2e tests for CAPI and external providers.

[e2e development]: ./e2e.md
