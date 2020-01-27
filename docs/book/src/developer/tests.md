# Running Cluster API tests

## Unit tests

Unit tests run very quickly. They don't require any additional services and can be run using default go tools or through
the `test` make target, e.g. `make test`.

## Integration tests

Integration tests use a real cluster and real dependencies to run tests. The dependencies are managed manually and are
not meant to be run locally. See `scripts/ci-integration.sh` for more details.

## End-to-end tests

The end-to-end tests are similar to the integration tests except they are designed to manage dependencies for you and 
have a more complete test using a docker provider to test the Cluster API mechanisms. The end-to-end tests run best on 
CI, but work locally if you're looking for early signal.

To run the tests, first, build the images needed for Cluster API from your local checkout. Then build the docker 
provider manager image. After that you can run the tests directly from make. Here are the commands to do the above steps. 

```
make docker-build REGISTRY=gcr.io/k8s-staging-cluster-api
make -C test/infrastructure/docker docker-build REGISTRY=gcr.io/k8s-staging-capi-docker
make -C test/infrastructure/docker run-e2e
```

These images only need to be built once. They can be reused if nothing changes. However, if a manager for a particular
image changes then the docker image will need to be rebuilt to see the changes reflected in the tests.

These end-to-end tests can be configured with a configuration file. Please take a look at our framework for more details.
