# Updating a v1alpha1 provider to a v1alpha2 infrastructure provider

This document outlines how to update a cluster API (CAPI) v1alpha1 provider to a v1alpha2 infrastructure provider.

<details>
<summary>
<strong>Table of contents</strong>
</summary>

* [Updating a v1alpha1 provider to a v1alpha2 infrastructure provider](#updating-a-v1alpha1-provider-to-a-v1alpha2-infrastructure-provider)
  * [General information](#general-information)
    * [Provider types](#provider-types)
      * [Providers in v1alpha1](#providers-in-v1alpha1)
      * [Providers in v1alpha2](#providers-in-v1alpha2)
    * [`clusterctl`](#clusterctl)
      * [`clusterctl` in v1alpha1](#clusterctl-in-v1alpha1)
      * [`clusterctl` in v1alpha2](#clusterctl-in-v1alpha2)
    * [The new API groups](#the-new-api-groups)
    * [Kubebuilder](#kubebuilder)
    * [Sample code and other examples](#sample-code-and-other-examples)
  * [Create a branch for new v1alpha1 work](#create-a-branch-for-new-v1alpha1-work)
  * [Update the API group in the `PROJECT` file](#update-the-api-group-in-the-project-file)
  * [Create the provider's v1alpha2 resources](#create-the-providers-v1alpha2-resources)
    * [The cluster and machine resources](#the-cluster-and-machine-resources)
    * [The spec and status types](#the-spec-and-status-types)
      * [Infrastructure provider cluster status fields](#infrastructure-provider-cluster-status-fields)
        * [Infrastructure provider cluster status `ready`](#infrastructure-provider-cluster-status-ready)
        * [Infrastructure provider cluster status `apiEndpoints`](#infrastructure-provider-cluster-status-apiendpoints)
  * [Create the infrastructure controllers](#create-the-infrastructure-controllers)
    * [The infrastructure provider cluster controller](#the-infrastructure-provider-cluster-controller)
    * [The infrastructure provider machine controller](#the-infrastructure-provider-machine-controller)

</details>

## General information

This section contains several general notes about the update process.

### Provider types

This section reviews the changes to the provider types from v1alpha1 to v1alpha2.

#### Providers in v1alpha1

Providers in v1alpha1 wrap behavior specific to an environment to create the infrastructure and bootstrap instances into
Kubernetes nodes. Examples of environments that have integrated with Cluster API v1alpha1 include, AWS, GCP, OpenStack,
Azure, vSphere and others. The provider vendors in Cluster API's controllers, registers its own actuators with the
Cluster API controllers and runs a single manager to complete a Cluster API management cluster.

#### Providers in v1alpha2

v1alpha2 introduces two new providers and changes how the Cluster API is consumed. This means that in order to have a
complete management cluster that is ready to build clusters you will need three managers.

* Core (Cluster API)
* Bootstrap (kubeadm)
* Infrastructure (aws, gcp, azure, vsphere, etc)

Cluster API's controllers are no longer vendored by providers. Instead, Cluster API offers its own independent
controllers that are responsible for the core types:

* Cluster
* Machine
* MachineSet
* MachineDeployment

Bootstrap providers are an entirely new concept aimed at reducing the amount of kubeadm boilerplate that every provider
reimplemented in v1alpha1. The Bootstrap provider is responsible for running a controller that generates data necessary
to bootstrap an instance into a Kubernetes node (cloud-init, bash, etc).

v1alpha1 "providers" have become Infrastructure providers. The Infrastructure provider is responsible for generating
actual infrastructure (networking, load balancers, instances, etc) for a particular environment that can consume
bootstrap data to turn the infrastructure into a Kubernetes cluster.

### `clusterctl`

The `clusterctl` program is also handled differently in v1alpha2.

#### `clusterctl` in v1alpha1

`clusterctl` was a command line tool packaged with v1alpha1 providers. The goal of this tool was to go from nothing to a
running management cluster in whatever environment the provider was built for. For example, Cluster-API-Provider-AWS
packaged a `clusterctl` that created a Kubernetes cluster in EC2 and installed the necessary controllers to respond to
Cluster API's APIs.

#### `clusterctl` in v1alpha2

`clusterctl` is likely becoming provider-agnostic meaning one clusterctl is bundled with Cluster API and can be reused
across providers. Work here is still being figured out but providers will not be packaging their own `clusterctl`
anymore.

### The new API groups

This section describes the API groups used by CAPI v1alpha2:

| Group | Description |
|---|---|
| `cluster.x-k8s.io` | The root CAPI API group |
| `infrastructure.cluster.x-k8s.io` | The API group for all resources related to CAPI infrastructure providers |
| `bootstrap.cluster.x-k8s.io` | The API group for all resources related to CAPI bootstrap providers |

Only SIG-sponsored providers may declare their components or resources to belong to any API group that ends with `x-k8s.io`.

Externally owned providers should use an appropriate API group for their ownership and would require additional RBAC rules to be configured and deployed for the common cluster-api components.

### Kubebuilder

While [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) v2 is available, the recommended approach for updating a CAPI provider to v1alpha2 is to stick with kubebuilder v1 during the update process and then reevaluate kubebuilder v2 after a successful migration to CAPI v1alpha2.

Please note if webhooks are required, it may necessitate migrating to kubebuilder v2 as part of the initial migration.

Additionally, kubebuilder v2 includes a lot of changes apart from just code, ex. the `Makefile`. If moving to v2 as part of this migration, please take a moment to read the kubebuilder book to understand all [the differences between v1 and v2](https://book.kubebuilder.io/migration/v1vsv2.html).

### Sample code and other examples

This document uses the CAPI provider for AWS ([CAPA](https://github.com/kubernetes-sigs/cluster-api-provider-aws)) for sample code and other examples.

## Create a branch for new v1alpha1 work

This document assumes the work required to update a provider to v1alpha2 will occur on the project's `master` branch. Therefore, the recommendation is to create a branch `release-MAJOR.MINOR` in the repository from the latest v1alpha1-based release. For example, if the latest release of a provider based on CAPI v1alpha1 was `v0.4.1` then the branch `release-0.4` should be created. Now the project's `master` branch is free to be a target for the work required to update the provider to v1alpha2, and fixes or backported features for the v1alpha1 version of the provider may target the `release-0.4` branch.

## Update the API group in the `PROJECT` file

Please update the `PROJECT` file at the root of the provider's repository to reflect the API group `cluster.x-k8s.io`:

```properties
version: "1"
domain: cluster.x-k8s.io
repo: sigs.k8s.io/cluster-api-provider-aws
```

## Create the provider's v1alpha2 resources

The new v1alpha2 types are located in `pkg/apis/infrastructure/v1alpha2`.

### The cluster and machine resources

Providers no longer store configuration and status data for clusters and machines in the CAPI `Cluster` and `Machine` resources. Instead, this information is stored in two, new, provider-specific CRDs:

* `pkg/apis/infrastructure/v1alpha2.`_Provider_`Cluster`
* `pkg/apis/infrastructure/v1alpha2.`_Provider_`Machine`

For example, the AWS provider defines:

* [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSCluster`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awscluster_types.go#L138-L146)
* [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSMachine`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awsmachine_types.go#L144-L152)

### The spec and status types

The `Spec` and `Status` types used to store configuration and status information are effectively the same in v1alpha2 as they were in v1alpha1:

| v1alpha1 | v1alpha2 |
|---|---|
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1alpha1.AWSClusterProviderSpec`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/apis/awsprovider/v1alpha1/awsclusterproviderconfig_types.go#L30-L65) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSClusterSpec`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awscluster_types.go#L33-L43) |
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1alpha1.AWSClusterProviderStatus`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/apis/awsprovider/v1alpha1/awsclusterproviderstatus_types.go#L26-L35) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSClusterStatus`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awscluster_types.go#L116-L124) |
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1alpha1.AWSMachineProviderSpec`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/apis/awsprovider/v1alpha1/awsmachineproviderconfig_types.go#L28-L97) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSMachineSpec`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awsmachine_types.go#L31-L87) |
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1alpha1.AWSMachineProviderStatus`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/apis/awsprovider/v1alpha1/awsmachineproviderstatus_types.go#L26-L44) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/apis/infrastructure/v1alpha2.AWSMachineStatus`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/6de25b31def9b4203a3c0a92b868a1819ea6e3e7/pkg/apis/infrastructure/v1alpha2/awsmachine_types.go#L89-L139) |

Information related to kubeadm or certificates has been extracted and is now owned by the bootstrap provider and its corresponding resources, ex. `KubeadmConfig`.

#### Infrastructure provider cluster status fields

A CAPI v1alpha2 provider cluster status resource has two special fields, `ready` and `apiEndpoints`. For example, take the `AWSClusterStatus`:

```golang
// AWSClusterStatus defines the observed state of AWSCluster
type AWSClusterStatus struct {
    Ready   bool     `json:"ready"`
    // APIEndpoints represents the endpoints to communicate with the control plane.
    // +optional
    APIEndpoints []APIEndpoint `json:"apiEndpoints,omitempty"`
}
```

##### Infrastructure provider cluster status `ready`

A Provider`Cluster`'s `status` object must define a boolean field named `ready` and set the value to `true` only when the infrastructure required to provision a cluster is ready and available.

##### Infrastructure provider cluster status `apiEndpoints`

A Provider`Cluster`'s `status` object may optionally define a field named `apiEndpoints` that is a list of the following objects:

```golang
// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
    // The hostname on which the API server is serving.
    Host string `json:"host"`

    // The port on which the API server is serving.
    Port int `json:"port"`
}
```

If present, this field is automatically inspected in order to obtain an endpoint at which the Kubernetes cluster may be accessed.

## Create the infrastructure controllers

The actuator model from v1alpha1 has been replaced by the infrastructure controllers in v1alpha2:

| v1alpha1 | v1alpha2 |
|---|---|
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/aws/actuators/cluster.Actuator`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/cloud/aws/actuators/cluster/actuator.go#L50-L57) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/controller/awscluster.ReconcileAWSCluster`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L98-L103) |
| [`sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/aws/actuators/machine.Actuator`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e6a57dc61826b8c7806eba22a513c9722c420754/pkg/cloud/aws/actuators/machine/actuator.go#L57-L65) | [`sigs.k8s.io/cluster-api-provider-aws/pkg/controller/awsmachine.ReconcileAWSMachine`](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awsmachine/awsmachine_controller.go#L104-L109) |

### The infrastructure provider cluster controller

Instead of processing the CAPI `Cluster` resources like the old actuator model, the new provider cluster controller operates on the new provider `Cluster` CRD. However, the overall workflow should feel the same as the old cluster actuator. For example, take the `AWSCluster` controller's [reconcile function](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L105-L162), it:

1. Fetches the [`AWSCluster` resource](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L113-L121):

    ```golang
    awsCluster := &infrastructurev1alpha2.AWSCluster{}
    err := r.Get(ctx, request.NamespacedName, awsCluster)
    if err != nil {
        if apierrors.IsNotFound(err) {
            return reconcile.Result{}, nil
        }
        return reconcile.Result{}, err
    }
    ```

2. [Fetches the CAPI cluster resource](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L125-L133) that has a one-to-one relationship with the `AWSCluster` resource:

    ```golang
    cluster, err := util.GetOwnerCluster(ctx, r.Client, awsCluster.ObjectMeta)
    if err != nil {
        return reconcile.Result{}, err
    }
    if cluster == nil {
        logger.Info("Waiting for Cluster Controller to set OwnerRef on AWSCluster")
        return reconcile.Result{}, nil
    }
    ```

    If the `AWSCluster` resource does not have a corresponding CAPI cluster resource then the reconcile will be reissued once the OwnerRef is assigned to the `AWSCluster` resource by the CAPI controller, triggering a new reconcile event.

3. Uses a `defer` statement to [ensure the `AWSCluster` resource is always patched](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L148-L153) back to the API server:

    ```golang
    defer func() {
        if err := clusterScope.Close(); err != nil && reterr == nil {
            reterr = err
        }
    }()
    ```

4. Handles both [deleted and non-deleted](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/0a05127734a4fb955742b27c6e326a65821851ce/pkg/controller/awscluster/awscluster_controller.go#L155-L161) clusters resources:

    ```golang
    // Handle deleted clusters
    if !awsCluster.DeletionTimestamp.IsZero() {
        return reconcileDelete(clusterScope)
    }

    // Handle non-deleted clusters
    return reconcileNormal(clusterScope)
    ```

### The infrastructure provider machine controller

The new provider machine controller is a slightly larger departure from the v1alpha1 machine actuator. This is because the machine actuator was based around a _Create_, _Read_, _Update_, _Delete_ (CRUD) model. Providers implementing the v1alpha1 machine actuator would implement each of those four functions. However, this was just an abstract way to represent a Kubernetes controller's reconcile loop.

The new, v1alpha2, provider machine controller merely takes the same CRUD model from the v1alpha1 machine actuator and applies it to a Kubernetes reconcile activity. The CAPI provider for vSphere (CAPV) actually includes a diagram that illustrates the v1alpha1 machine actuator CRUD operations as a reconcile loop.

![CAPV machine reconcile](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-vsphere/28e01e4d1037c63970181a81378f38b294972c14/docs/design/machine-controller-reconcile.svg?sanitize=true)
