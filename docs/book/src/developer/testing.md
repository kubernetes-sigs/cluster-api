# Testing Cluster API

This document presents testing guidelines and conventions for Cluster API.

IMPORTANT: improving and maintaining this document is a collaborative effort, so we are encouraging constructive
feedback and suggestions.

## Unit tests

Unit tests focus on individual pieces of logic - a single func - and don't require any additional services to execute. They should
be fast and great for getting the first signal on the current implementation, but unit tests have the risk of
allowing integration bugs to slip through.

Historically, in Cluster API unit tests were developed using [go test], [gomega] and the [fakeclient]; see the quick reference below.

However, considering some changes introduced in the v0.3.x releases (e.g. ObservedGeneration, Conditions), there is a common
agreement among Cluster API maintainers that using [fakeclient] should be progressively deprecated in favor of using
[envtest]. See the quick reference below.

## Integration tests

Integration tests are focused on testing the behavior of an entire controller or the interactions between two or
more Cluster API controllers.

In older versions of Cluster API, integration tests were based on a real cluster and meant to be run in CI only; however,
now we are considering a different approach based on [envtest] and with one or more controllers configured to run against
the test cluster.

With this approach it is possible to interact with Cluster API like in a real environment, by creating/updating
Kubernetes objects and waiting for the controllers to take action.

Please note that while using this mode, as of today, when testing the interactions with an infrastructure provider
some infrastructure components will be generated, and this could have relevant impacts on test durations (and requirements).

While, as of today this is a strong limitation, in the future we might consider to have a "dry-run" option in CAPD or
a fake infrastructure provider to allow test coverage for testing the interactions with an infrastructure provider as well.

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
  different infrastructure providers (see [Writing Portable Tests](#writing-portable-e2e-tests)).

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
                "ARTIFACTS":"${workspaceRoot}/_artifacts",
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

If you want to debug CAPI controller during e2e tests, just scale down the controller in the local kind cluster
and run it via the IDE.

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
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		if err := (&MyReconciler{
			Client:  mgr.GetClient(),
			Log:     log.NullLogger{},
		}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start the MyReconciler: %v", err))
		}
	}

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}
```

Most notably, [envtest] provides not only a real API server to use during testing, but it offers the opportunity
to configure one or more controllers to run against the test cluster. By using this feature it is possible to use
[envtest] for developing Cluster API integration tests.

```golang
func TestMain(m *testing.M) {
	// Bootstrapping test environment
	...

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		if err := (&MyReconciler{
			Client:  mgr.GetClient(),
			Log:     log.NullLogger{},
		}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start the MyReconciler: %v", err))
		}
	}

	// Run tests
	...
}
```

Please note that, because [envtest] uses a real kube-apiserver that is shared across many tests, the developer
should take care in ensuring each test runs in isolation from the others, by:

- Creating objects in separated namespaces.
- Avoiding object name conflict.

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
- API calls doe not execute defaulting or validation webhooks, so there are no enforced guarantees about the semantic accuracy
  of the test objects.

Historically, [fakeclient] is widely used in Cluster API, however, given the growing relevance of the above limitations
with regard to some changes introduced in the v0.3.x releases (e.g. ObservedGeneration, Conditions), there is a common
agreement among Cluster API maintainers that using [fakeclient] should be progressively deprecated in favor of use
of [envtest].

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

[Cluster API quick start]: https://cluster-api.sigs.k8s.io/user/quick-start.html
[Cluster API test framework]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc
[e2e development]: ./e2e.md
[Ginkgo]: http://onsi.github.io/ginkgo/
[Gomega]: http://onsi.github.io/gomega/
[go test]: https://golang.org/pkg/testing/
[controller-runtime]: https://github.com/kubernetes-sigs/controller-runtime
[envtest]: https://github.com/kubernetes-sigs/controller-runtime/tree/master/pkg/envtest
[fakeclient]: https://github.com/kubernetes-sigs/controller-runtime/tree/master/pkg/client/fake
[test/helpers]: https://github.com/kubernetes-sigs/cluster-api/tree/main/test/helpers
