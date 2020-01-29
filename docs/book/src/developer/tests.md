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
CI, but work locally as well.

### Running the tests

`make test-capd-e2e-full` will build all manifests and provider images then use those manifests and images in the test
run. This is great if you're modifying API types in any of the providers.

`make test-capd-e2e-images` will only build the images for all providers then use those images during the e2e tests and
use whatever manifests exist on disk. This is good if you're only updating controller code.

`make test-capd-e2e` will only build the docker provider image and use whatever provider images already exist on disk.
This is good if you're working on the test framework itself. You'll likely want to build the images at least once
and then use this for a faster test run.
