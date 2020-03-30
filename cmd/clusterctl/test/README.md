# Clusterctl E2E tests

The e2e test framework is driven by a config file, so you can easily switch images/manifests from dev/ci or prod,
 switch the infrastructure provider, use different workload cluster templates.

The entire test suite can be run using the script:

	make -C cmd/clusterctl/test/e2e/ test-e2e

To run specific suite/spec, use the `GINKGO_FOCUS` os variable:

	make -C cmd/clusterctl/test/e2e/ test-e2e GINKGO_FOCUS="init"

To select a specific test configuration, use the `E2E_CONF_FILE` os variable:

	cmd/clusterctl/test/run-e2e.sh E2E_CONF_FILE=~/my-e2e.conf

To specify where test artifacts should be stored, use the `ARTIFACTS` os variable:

	make -C cmd/clusterctl/test/e2e/ test-e2e ARTIFACTS=~/_artifacts

To skip resource cleanup, use the `SKIP_RESOURCE_CLEANUP` os variable:

    make -C cmd/clusterctl/test/e2e/ test-e2e SKIP_RESOURCE_CLEANUP=true