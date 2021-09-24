# Developing E2E tests

E2E tests are meant to verify the proper functioning of a Cluster API management
cluster in an environment that resembles a real production environment.

The following guidelines should be followed when developing E2E tests:

- Use the [Cluster API test framework].
- Define test spec reflecting real user workflow, e.g. [Cluster API quick start].
- Unless you are testing provider specific features, ensure your test can run with
  different infrastructure providers (see [Writing Portable Tests](#writing-portable-e2e-tests)).

The [Cluster API test framework] provides you a set of helper methods for getting your test in place
quickly. The [test E2E package] provides examples of how this can be achieved and reusable
test specs for the most common Cluster API use cases.

## Prerequisites

Each E2E test requires a set of artifacts to be available:

- Binaries & docker images for Kubernetes, CNI, CRI & CSI
- Manifests & docker images for the Cluster API core components
- Manifests & docker images for the Cluster API infrastructure provider; in most cases
  machine images are also required (AMI, OVA etc.)
- Credentials for the target infrastructure provider
- Other support tools (e.g. kustomize, gsutil etc.)

The Cluster API test framework provides support for building and retrieving the manifest
files for Cluster API core components and for the Cluster API infrastructure provider
(see [Setup](#setup)).

For the remaining tasks you can find examples of
how this can be implemented e.g. in [CAPA E2E tests] and [CAPG E2E tests].

## Setup

In order to run E2E tests it is required to create a Kubernetes cluster with a
complete set of Cluster API providers installed. Setting up those elements is
usually implemented in a `BeforeSuite` function, and it consists of two steps:

- Defining an E2E config file
- Creating the management cluster and installing providers

### Defining an E2E config file

The [E2E config file] provides a convenient and flexible way to define common tasks for
setting up a management cluster.

Using the config file it is possible to:

- Define the list of providers to be installed in the management cluster. Most notably,
  for each provider it is possible to define:
  - One or more versions of the providers manifest (built from the sources, or pulled from a
    remote location).
  - A list of additional files to be added to the provider repository, to be used e.g.
    to provide `cluster-templates.yaml` files.
- Define the list of variables to be used when doing `clusterctl init` or
  `clusterctl generate cluster`.
- Define a list of intervals to be used in the test specs for defining timeouts for the
  wait and `Eventually` methods.
- Define the list of images to be loaded in the management cluster (this is specific to
  management clusters based on kind).

An [example E2E config file] can be found here.

### Creating the management cluster and installing providers

In order to run Cluster API E2E tests, you need a Kubernetes cluster. The [NewKindClusterProvider] gives you a
type that can be used to create a local kind cluster and pre-load images into it. Existing clusters can
be used if available.

Once you have a Kubernetes cluster, the [InitManagementClusterAndWatchControllerLogs method] provides a convenient
way for installing providers.

This method:
- Runs `clusterctl init` using the above local repository.
- Waits for the providers controllers to be running.
- Creates log watchers for all the providers

<aside class="note">

<h1>Deprecated InitManagementCluster method</h1>

The [Cluster API test framework] also includes a [deprecated InitManagementCluster method] implementation,
that was used before the introduction of clusterctl. This might be removed in future releases
of the test framework.

</aside>

## Writing test specs

A typical test spec is a sequence of:

- Creating a namespace to host in isolation all the test objects.
- Creating objects in the management cluster, wait for the corresponding infrastructure to be provisioned.
- Exec operations like e.g. changing the Kubernetes version or  `clusterctl move`, wait for the action to complete.
- Delete objects in the management cluster, wait for the corresponding infrastructure to be terminated.

### Creating Namespaces

The [CreateNamespaceAndWatchEvents method] provides a convenient way to create a namespace and setup
watches for capturing namespaces events.

### Creating objects

There are two possible approaches for creating objects in the management cluster:

- Create object by object: create the `Cluster` object, then `AwsCluster`, `Machines`, `AwsMachines` etc.
- Apply a `cluster-templates.yaml` file thus creating all the objects this file contains.

The first approach leverages the [controller-runtime Client] and gives you full control, but it comes with
some drawbacks as well, because this method does not directly reflect real user workflows, and most importantly,
the resulting tests are not as reusable with other infrastructure providers. (See [writing portable tests](#writing-portable-e2e-tests)).

We recommend using the [ClusterTemplate method] and the [Apply method] for creating objects in the cluster.
This methods mimics the recommended user workflows, and it is based on `cluster-templates.yaml` files that can be
provided via the [E2E config file], and thus easily swappable when changing the target infrastructure provider.

<aside class="note">

<h1>Tips</h1>

If you need control over object creation but want to preserve portability, you can create many templates
files each one creating only a small set of objects (instead of using a single template creating a full cluster).

</aside>

After creating objects in the cluster, use the existing methods in the [Cluster API test framework] to discover
which object were created in the cluster so your code can adapt to different `cluster-templates.yaml` files.

Once you have object references, the framework includes methods for waiting for the corresponding
infrastructure to be provisioned, e.g. [WaitForClusterToProvision], [WaitForKubeadmControlPlaneMachinesToExist].

### Exec operations

You can use [Cluster API test framework] methods to modify Cluster API objects, as a last option, use
the [controller-runtime Client].

The [Cluster API test framework] also includes methods for executing clusterctl operations, like e.g.
the [ClusterTemplate method], the [ClusterctlMove method] etc.. In order to improve observability,
each clusterctl operation creates a detailed log.

After using clusterctl operations, you can rely on the `Get` and on the `Wait` methods
defined in the [Cluster API test framework] to check if the operation completed successfully.

### Naming the test spec

You can categorize the test with a custom label that can be used to filter a category of E2E tests to be run. Currently, the cluster-api codebase has [these labels](./testing.html#running-specific-tests) which are used to run a focused subset of tests.

## Tear down

After a test completes/fails, it is required to:

- Collect all the logs for the Cluster API controllers
- Dump all the relevant Cluster API/Kubernetes objects
- Cleanup all the infrastructure resources created during the test

Those tasks are usually implemented in the `AfterSuite`, and again the [Cluster API test framework] provides
you useful methods for those tasks.

Please note that despite the fact that test specs are expected to delete objects in the management cluster and
wait for the corresponding infrastructure to be terminated, it can happen that the test spec
fails before starting object deletion or that objects deletion itself fails.

As a consequence, when scheduling/running a test suite, it is required to ensure all the generated
resources are cleaned up. In Kubernetes, this is implemented by the [boskos] project.

## Writing portable E2E tests

A portable E2E test is a test that can run with different infrastructure providers by simply
changing the test configuration file.

The following recommendations should be followed to write portable E2E tests:

- Create different [E2E config file], one for each target infrastructure provider,
  providing different sets of env variables and timeout intervals.
- Use the [InitManagementCluster method] for setting up the management cluster.
- Use the [ClusterTemplate method] and the [Apply method]
  for creating objects in the cluster using `cluster-templates.yaml` files instead
  of hard coding object creation.
- Use the `Get` methods defined in the [Cluster API test framework] to check objects
  being created, so your code can adapt to different `cluster-templates.yaml` files.
- Never hard code the infrastructure provider name in your test spec.
  Instead, use the [InfrastructureProvider method] to get access to the
  name of the infrastructure provider defined in the [E2E config file].
- Never hard code wait intervals in your test spec.
  Instead use the [GetIntervals method] to get access to the
  intervals defined in the [E2E config file].

## Cluster API conformance tests

As of today there is no a well-defined suite of E2E tests that can be used as a
baseline for Cluster API conformance.

However, creating such a suite is something that can provide a huge value for the
long term success of the project.

The [test E2E package] provides examples of how this can be achieved by implementing a set of reusable
test specs for the most common Cluster API use cases.

<!-- links -->
[Cluster API quick start]: https://cluster-api.sigs.k8s.io/user/quick-start.html
[Cluster API test framework]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc
[deprecated E2E config file]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#Config
[deprecated InitManagementCluster method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#InitManagementCluster
[Apply method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#Applier
[CAPA E2E tests]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/scripts/ci-e2e.sh
[CAPG E2E tests]: https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/scripts/ci-e2e.sh
[WaitForClusterToProvision]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#WaitForClusterToProvision
[WaitForKubeadmControlPlaneMachinesToExist]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#WaitForKubeadmControlPlaneMachinesToExist
[controller-runtime Client]: https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.5.2/pkg/client?tab=doc#Client
[boskos]: https://git.k8s.io/test-infra/boskos
[E2E config file]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#E2EConfig
[example E2E config file]: https://github.com/kubernetes-sigs/cluster-api/blob/main/test/e2e/config/docker.yaml
[NewKindClusterProvider]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/bootstrap?tab=doc#NewKindClusterProvider
[InitManagementClusterAndWatchControllerLogs method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#InitManagementClusterAndWatchControllerLogs
[ClusterTemplate method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#ConfigCluster
[ClusterctlMove method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#Move
[InfrastructureProvider method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#E2EConfig.InfrastructureProviders
[GetIntervals method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework/clusterctl?tab=doc#E2EConfig.GetIntervals
[test E2E package]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/e2e?tab=doc
[CreateNamespaceAndWatchEvents method]: https://pkg.go.dev/sigs.k8s.io/cluster-api/test/framework?tab=doc#CreateNamespaceAndWatchEvents
