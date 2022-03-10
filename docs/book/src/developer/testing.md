# Testing Cluster API

This document presents testing guidelines and conventions for Cluster API.

IMPORTANT: improving and maintaining this document is a collaborative effort, so we are encouraging constructive
feedback and suggestions.

## Unit tests

Unit tests focus on individual pieces of logic - a single func - and don't require any additional services to execute. They should
be fast and great for getting the first signal on the current implementation, but unit tests have the risk of
allowing integration bugs to slip through.

In Cluster API most of the unit tests are developed using [go test], [gomega] and the [fakeclient]; however using
[fakeclient] is not suitable for all the use cases due to some limitations in how it is implemented. In some cases
contributors will be required to use [envtest]. See the [quick reference](#quick-reference) below for more details.

### Mocking external APIs
In some cases when writing tests it is required to mock external API, e.g. etcd client API or the AWS SDK API.

This problem is usually well scoped in core Cluster API, and in most cases it is already solved by using fake
implementations of the target API to be injected during tests.

Instead, mocking is much more relevant for infrastructure providers; in order to address the issue
some providers can use simulators reproducing the behaviour of a real infrastructure providers (e.g CAPV);
if this is not possible, a viable solution is to use mocks (e.g CAPA).

### Generic providers
When writing tests core Cluster API contributors should ensure that the code works with any providers, and thus it is required
to not use any specific provider implementation. Instead, the so-called generic providers e.g. "GenericInfrastructureCluster" 
should be used because they implement the plain Cluster API contract. This prevents tests from relying on assumptions that 
may not hold true in all cases.

Please note that in the long term we would like to improve the implementation of generic providers, centralizing
the existing set of utilities scattered across the codebase, but while details of this work will be defined do not
hesitate to reach out to reviewers and maintainers for guidance.

## Integration tests

Integration tests are focused on testing the behavior of an entire controller or the interactions between two or
more Cluster API controllers.

In Cluster API, integration tests are based on [envtest] and one or more controllers configured to run against
the test cluster.

With this approach it is possible to interact with Cluster API almost like in a real environment, by creating/updating
Kubernetes objects and waiting for the controllers to take action. See the [quick reference](#quick-reference) below for more details.

Also in case of integration tests, considerations about [mocking external APIs](#mocking-external-apis) and usage of [generic providers](#generic-providers) apply. 

## Test maintainability

Tests are an integral part of the project codebase.

Cluster API maintainers and all the contributors should be committed to help in ensuring that tests are easily maintainable,
easily readable, well documented and consistent across the code base.

In light of continuing improving our practice around this ambitious goal, we are starting to introduce a shared set of:

- Builders (`sigs.k8s.io/cluster-api/internal/test/builder`), allowing to create test objects in a simple and consistent way.
- Matchers (`sigs.k8s.io/cluster-api/internal/test/matchers`), improving how we write test assertions.

Each contribution in growing this set of utilities or their adoption across the codebase is more than welcome!

Another consideration that can help in improving test maintainability is the idea of testing "by layers"; this idea could 
apply whenever we are testing "higher-level" functions that internally uses one or more "lower-level" functions;
in order to avoid writing/maintaining redundant tests, whenever possible contributors should take care of testing
_only_ the logic that is implemented in the "higher-level" function, delegating the test function called internally
to a "lower-level" set of unit tests.

A similar concern could be raised also in the case whenever there is overlap between unit tests and integration tests,
but in this case the distinctive value of the two layers of testing is determined by how test are designed:

- unit test are focused on code structure: func(input) = output, including edge case values, asserting error conditions etc.
- integration test are user story driven: as a user, I want express some desired state using API objects, wait for the
  reconcilers to take action, check the new system state.

## Running unit and integration tests

Run `make test` to execute all unit and integration tests.

<aside class="note">

<h1>Tips</h1>

When testing individual packages, you can speed up the test execution by running the tests with a local kind cluster.
This avoids spinning up a testenv with each test execution. It also makes it easier to debug, because it's straightforward
to access a kind cluster with kubectl during test execution. For further instructions, run: `./hack/setup-envtest-with-kind.sh`.

When running individual tests, it could happen that a testenv is started if this is required by the `suite_test.go` file.
However, if the tests you are running don't require testenv (i.e. they are only using fake client), you can skip the testenv
creation by setting the environment variable `CAPI_DISABLE_TEST_ENV` (to any non-empty value).

</aside>

## End-to-end tests

The end-to-end tests are meant to verify the proper functioning of a Cluster API management cluster
in an environment that resemble a real production environment.

The following guidelines should be followed when developing E2E tests:

- Use the [Cluster API test framework].
- Define test spec reflecting real user workflow, e.g. [Cluster API quick start].
- Unless you are testing provider specific features, ensure your test can run with
  different infrastructure providers (see [Writing Portable Tests](./e2e.md#writing-portable-e2e-tests)).

See [e2e development] for more information on developing e2e tests for CAPI and external providers.

## Running the end-to-end tests locally

Usually the e2e tests are executed by Prow, either pre-submit (on PRs) or periodically on certain branches
(e.g. the default branch). Those jobs are defined in the kubernetes/test-infra repository in [config/jobs/kubernetes-sigs/cluster-api](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api).
For development and debugging those tests can also be executed locally.

### Prerequisites

`make docker-build-e2e` will build the images for all providers that will be needed for the e2e tests.

### Test execution via ci-e2e.sh

To run a test locally via the command line, you should look at the Prow Job configuration for the test you want to run and then execute the same commands locally.
For example to run [pull-cluster-api-e2e-main](https://github.com/kubernetes/test-infra/blob/49ab08a6a2a17377d52a11212e6f1104c3e87bfc/config/jobs/kubernetes-sigs/cluster-api/cluster-api-presubmits-main.yaml#L113-L140)
just execute:

```bash
GINKGO_FOCUS="\[PR-Blocking\]" ./scripts/ci-e2e.sh
```

### Test execution via make test-e2e

`make test-e2e` will run e2e tests by using whatever provider images already exist on disk.
After running `make docker-build-e2e` at least once, `make test-e2e` can be used for a faster test run, if there are no
provider code changes. If the provider code is changed, run `make docker-build-e2e` to update the images.

### Test execution via IDE

It's also possible to run the tests via an IDE which makes it easier to debug the test code by stepping through the code.

First, we have to make sure all prerequisites are fulfilled, i.e. all required images have been built (this also includes
kind images). This can be done by executing the `./scripts/ci-e2e.sh` script.

```bash
# Notes:
# * You can cancel the script as soon as it starts the actual test execution via `make -C test/e2e/ run`.
# * If you want to run other tests (e.g. upgrade tests), make sure all required env variables are set (see the Prow Job config).
GINKGO_FOCUS="\[PR-Blocking\]" ./scripts/ci-e2e.sh

# Make sure the cluster-templates have been generated.
make -C test/e2e cluster-templates
```

Now, the tests can be run in an IDE. The following describes how this can be done in Intellij IDEA and VS Code. It should work
roughly the same way in all other IDEs. We assume the `cluster-api` repository has been checked
out into `/home/user/code/src/sigs.k8s.io/cluster-api`.

#### Intellij

Create a new run configuration and fill in:
* Test framework: `gotest`
* Test kind: `Package`
* Package path: `sigs.k8s.io/cluster-api/test/e2e`
* Pattern: `^\QTestE2E\E$`
* Working directory: `/home/user/code/src/sigs.k8s.io/cluster-api/test/e2e`
* Environment: `ARTIFACTS=/home/user/code/src/sigs.k8s.io/cluster-api/_artifacts`
* Program arguments: `-e2e.config=/home/user/code/src/sigs.k8s.io/cluster-api/test/e2e/config/docker.yaml -ginkgo.focus="\[PR-Blocking\]"`

#### VS Code

Add the launch.json file in the .vscode folder in your repo:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Run e2e test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceRoot}/test/e2e/e2e_suite_test.go",
            "env": {
                "ARTIFACTS":"${workspaceRoot}/_artifacts"
            },
            "args": [
                "-e2e.config=${workspaceRoot}/test/e2e/config/docker.yaml",
                "-ginkgo.focus=\\[PR-Blocking\\]",
                "-ginkgo.v=true"
            ],
            "trace": "verbose",
            "buildFlags": "-tags 'e2e'",
            "showGlobalVariables": true
        }
    ]
}
```

Execute the run configuration with `Debug`.

<aside class="note">

<h1>Tips</h1>

The e2e tests create a new management cluster with kind on each run. To avoid this and speed up the test execution the tests can 
also be run against a management cluster created by [tilt](./tilt.md):
```bash
# Create a kind cluster
./hack/kind-install-for-capd.sh
# Set up the management cluster via tilt
tilt up 
```
Now you can start the e2e test via IDE as described above but with the additional `-e2e.use-existing-cluster=true` flag.

**Note**: This can also be used to debug controllers during e2e tests as described in [Developing Cluster API with Tilt](./tilt.md#wiring-up-debuggers).

The e2e tests also create a local clusterctl repository. After it has been created on a first test execution this step can also be 
skipped by setting `-e2e.cluster-config=<ARTIFACTS>/repository/clusterctl-config.yaml`. This also works with a clusterctl repository created 
via [Create the local repository](http://localhost:3000/clusterctl/developers.html#create-the-local-repository).

**Feature gates**: E2E tests often use features which need to be enabled first. Make sure to enable the feature gates in the tilt settings file:
```yaml
kustomize_substitutions:
  CLUSTER_TOPOLOGY: "true"
  EXP_MACHINE_POOL: "true"
  EXP_CLUSTER_RESOURCE_SET: "true"
  EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION: "true"
```

</aside>

### Running specific tests

To run a subset of tests, a combination of either one or both of `GINKGO_FOCUS` and `GINKGO_SKIP` env variables can be set.
Each of these can be used to match tests, for example:
- `[PR-Blocking]` => Sanity tests run before each PR merge
- `[K8s-Upgrade]` => Tests which verify k8s component version upgrades on workload clusters
- `[Conformance]` => Tests which run the k8s conformance suite on workload clusters
- `When testing KCP.*` => Tests which start with `When testing KCP`

For example:
` GINKGO_FOCUS="\\[PR-Blocking\\]" make test-e2e ` can be used to run the sanity E2E tests
` GINKGO_SKIP="\\[K8s-Upgrade\\]" make test-e2e ` can be used to skip the upgrade E2E tests

### Further customization

The following env variables can be set to customize the test execution:

- `GINKGO_FOCUS` to set ginkgo focus (default empty - all tests)
- `GINKGO_SKIP` to set ginkgo skip (default empty - to allow running all tests)
- `GINKGO_NODES` to set the number of ginkgo parallel nodes (default to 1)
- `E2E_CONF_FILE` to set the e2e test config file (default to ${REPO_ROOT}/test/e2e/config/docker.yaml)
- `ARTIFACTS` to set the folder where test artifact will be stored (default to ${REPO_ROOT}/_artifacts)
- `SKIP_RESOURCE_CLEANUP` to skip resource cleanup at the end of the test (useful for problem investigation) (default to false)
- `USE_EXISTING_CLUSTER` to use an existing management cluster instead of creating a new one for each test run (default to false)
- `GINKGO_NOCOLOR` to turn off the ginkgo colored output (default to false)

Furthermore, it's possible to overwrite all env variables specified in `variables` in `test/e2e/config/docker.yaml`.

## Troubleshooting end-to-end tests

### Analyzing logs

Logs of e2e tests can be analyzed with our development environment by pushing logs to Loki and then
analyzing them via Grafana.

1. Start the development environment as described in [Developing Cluster API with Tilt](./tilt.md).
    * Make sure to deploy Loki and Grafana via `deploy_observability`.
    * If you only want to see imported logs, don't deploy promtail (via `deploy_observability`).
    * If you want to drop all logs from Loki, just delete the Loki Pod in the `observability` namespace.
2. You can then import logs via the `Import Logs` button on the top right of the [Loki resource page](http://localhost:10350/r/loki/overview).
   Just click on the downwards arrow, enter either a ProwJob URL, a GCS path or a local folder and click on `Import Logs`.
   This will retrieve the logs and push them to Loki. Alternatively, the logs can be imported via:
   ```bash
   go run ./hack/tools/log-push --log-path=<log-path>
   ```
   Examples for log paths:
    * ProwJob URL: `https://prow.k8s.io/view/gs/kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-e2e-main/1496954690603061248`
    * GCS path: `gs://kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-e2e-main/1496954690603061248`
    * Local folder: `./_artifacts`
4. Now the logs are available:
    * via [Grafana](http://localhost:3001/explore)
    * via [Loki logcli](https://grafana.com/docs/loki/latest/getting-started/logcli/)
      ```bash
      logcli query '{app="capi-controller-manager"}' --timezone=UTC --from="2022-02-22T10:00:00Z"
      ```

<aside class="note">

<h1>Caveats</h1>

* Make sure you query the correct time range via Grafana or `logcli`.
* The logs are currently uploaded by using now as the timestamp, because otherwise it would
  take a few minutes until the logs show up in Loki.

</aside>

### Known Issues

#### Building images on SELinux

Cluster API repositories use [Moby Buildkit](https://github.com/moby/buildkit) to speed up image builds.
[BuildKit does not currently work on SELinux](https://github.com/moby/buildkit/issues/2295).

Use `sudo setenforce 0` to make SELinux permissive when running e2e tests.

## Quick reference

### `envtest`

[envtest] is a testing environment that is provided by the [controller-runtime] project. This environment spins up a
local instance of etcd and the kube-apiserver. This allows tests to be executed in an environment very similar to a
real environment.

Additionally, in Cluster API there is a set of utilities under [internal/envtest] that helps developers in setting up
a [envtest] ready for Cluster API testing, and more specifically:

- With the required CRDs already pre-configured.
- With all the Cluster API webhook pre-configured, so there are enforced guarantees about the semantic accuracy
  of the test objects you are going to create.

This is an example of how to create an instance of [envtest] that can be shared across all the tests in a package;
by convention, this code should be in a file named `suite_test.go`:

```golang
var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	// Setup envtest
	...

	// Run tests
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
```

Most notably, [envtest] provides not only a real API server to use during testing, but it offers the opportunity
to configure one or more controllers to run against the test cluster, as well as creating informers index. 

```golang
func TestMain(m *testing.M) {
	// Setup envtest
	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		if err := (&MyReconciler{
			Client:  mgr.GetClient(),
			Log:     log.NullLogger{},
		}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start the MyReconciler: %v", err))
		}
	}

	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
		panic(fmt.Sprintf("unable to setup index: %v", err))
	}
    
    // Run tests
	...
}
```

By combining pre-configured validation and mutating webhooks and reconcilers/indexes it is possible
to use [envtest] for developing Cluster API integration tests that can mimic how the system
behaves in real Cluster.

Please note that, because [envtest] uses a real kube-apiserver that is shared across many test cases, the developer
should take care in ensuring each test runs in isolation from the others, by:

- Creating objects in separated namespaces.
- Avoiding object name conflict.

Developers should also be aware of the fact that the informers cache used to access the [envtest]
depends on actual etcd watches/API calls for updates, and thus it could happen that after creating 
or deleting objects the cache takes a few milliseconds to get updated. This can lead to test flakes, 
and thus it always recommended to use patterns like create and wait or delete and wait; Cluster API env
test provides a set of utils for this scope.

However, developers should be aware that in some ways, the test control plane will behave differently from “real”
clusters, and that might have an impact on how you write tests.

One common example is garbage collection; because there are no controllers monitoring built-in resources, objects
do not get deleted, even if an OwnerReference is set up; as a consequence, usually test implements code for cleaning up
created objects.

This is an example of a test implementing those recommendations:

```golang
func TestAFunc(t *testing.T) {
	g := NewWithT(t)
	// Generate namespace with a random name starting with ns1; such namespace
	// will host test objects in isolation from other tests.
	ns1, err := env.CreateNamespace(ctx, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		// Cleanup the test namespace
		g.Expect(env.DeleteNamespace(ctx, ns1)).To(Succeed())
	}()

	obj := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: ns1.Name, // Place test objects in the test namespace
		},
	}

	// Actual test code...
}
```

In case of object used in many test case within the same test, it is possible to leverage on Kubernetes `GenerateName`;
For objects that are shared across sub-tests, ensure they are scoped within the test namespace and deep copied to avoid
cross-test changes that may occur to the object.

```golang
func TestAFunc(t *testing.T) {
	g := NewWithT(t)
	// Generate namespace with a random name starting with ns1; such namespace
	// will host test objects in isolation from other tests.
	ns1, err := env.CreateNamespace(ctx, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		// Cleanup the test namespace
		g.Expect(env.DeleteNamespace(ctx, ns1)).To(Succeed())
	}()

	obj := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",  // Instead of assigning a name, use GenerateName
			Namespace:    ns1.Name, // Place test objects in the test namespace
		},
	}

	t.Run("test case 1", func(t *testing.T) {
		g := NewWithT(t)
		// Deep copy the object in each test case, so we prevent side effects in case the object changes.
		// Additionally, thanks to GenerateName, the objects gets a new name for each test case.
		obj := obj.DeepCopy()

	    // Actual test case code...
	}
	t.Run("test case 2", func(t *testing.T) {
		g := NewWithT(t)
		obj := obj.DeepCopy()

	    // Actual test case code...
	}
	// More test cases.
}
```

### `fakeclient`

[fakeclient] is another utility that is provided by the [controller-runtime] project. While this utility is really
fast and simple to use because it does not require to spin-up an instance of etcd and kube-apiserver, the [fakeclient]
comes with a set of limitations that could hamper the validity of a test, most notably:

- it does not properly handle a set of fields which are common in the Kubernetes API objects (and Cluster API objects as well)
  like e.g. `creationTimestamp`, `resourceVersion`, `generation`, `uid`
- [fakeclient] operations do not trigger defaulting or validation webhooks, so there are no enforced guarantees about the semantic accuracy
  of the test objects.
- the [fakeclient] does not use a cache based on informers/API calls/etcd watches, so the test written in this way
  can't help in surfacing race conditions related to how those components behave in real cluster.
- there is no support for cache index/operations using cache indexes. 

Accordingly, using [fakeclient] is not suitable for all the use cases, so in some cases contributors will be required
to use [envtest] instead. In case of doubts about which one to use when writing tests, don't hesitate to ask for
guidance from project maintainers.

### `ginkgo`
[Ginkgo] is a Go testing framework built to help you efficiently write expressive and comprehensive tests using Behavior-Driven Development (“BDD”) style.

While [Ginkgo] is widely used in the Kubernetes ecosystem, Cluster API maintainers found the lack of integration with the
most used golang IDE somehow limiting, mostly because:

- it makes interactive debugging of tests more difficult, since you can't just run the test using the debugger directly
- it makes it more difficult to only run a subset of tests, since you can't just run or debug individual tests using an IDE,
  but you now need to run the tests using `make` or the `ginkgo` command line and override the focus to select individual tests

In Cluster API you MUST use ginkgo only for E2E tests, where it is required to leverage the support for running specs
in parallel; in any case, developers MUST NOT use the table driven extension DSL (`DescribeTable`, `Entry` commands)
which is considered unintuitive.

### `gomega`
[Gomega] is a matcher/assertion library. It is usually paired with the Ginkgo BDD test framework, but it can be used with
 other test frameworks too.

 More specifically, in order to use Gomega with go test you should

 ```golang
 func TestFarmHasCow(t *testing.T) {
     g := NewWithT(t)
     g.Expect(f.HasCow()).To(BeTrue(), "Farm should have cow")
 }
```

In Cluster API all the test MUST use [Gomega] assertions.

### `go test`

[go test] testing provides support for automated testing of Go packages.

In Cluster API Unit and integration test MUST use [go test].

[Cluster API quick start]: ../user/quick-start.md
[Cluster API test framework]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc
[e2e development]: ./e2e.md
[Ginkgo]: http://onsi.github.io/ginkgo/
[Gomega]: http://onsi.github.io/gomega/
[go test]: https://golang.org/pkg/testing/
[controller-runtime]: https://github.com/kubernetes-sigs/controller-runtime
[envtest]: https://github.com/kubernetes-sigs/controller-runtime/tree/master/pkg/envtest
[fakeclient]: https://github.com/kubernetes-sigs/controller-runtime/tree/master/pkg/client/fake
[test/helpers]: https://github.com/kubernetes-sigs/cluster-api/tree/main/test/helpers
