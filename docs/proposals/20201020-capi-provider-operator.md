---
title: CAPI Provider Operator
authors:
  - "@fabriziopandini"
  - "@wfernandes"
reviewers:
  - "@vincepri"
  - "@ncdc"
  - "@justinsb"
  - "@detiber"
  - "@CecileRobertMichon"
creation-date: 2020-09-14
last-updated: 2021-01-20
status: implementable
see-also:
https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20191016-clusterctl-redesign.md
---

# CAPI Provider operator

## Table of Contents

* [CAPI provider operator](#capi-provider-operator)
  * [Table of Contents](#table-of-contents)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
     * [Goals](#goals)
     * [Non-Goals/Future Work](#non-goalsfuture-work)
  * [Proposal](#proposal)
     * [User Stories](#user-stories)
     * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
        * [Existing API Types Changes](#existing-api-types-changes)
        * [New API Types](#new-api-types)
        * [Example API Usage](#example-api-usage)
        * [Operator Behaviors](#operator-behaviors)
           * [Installing a provider](#installing-a-provider)
           * [Upgrading a provider](#upgrading-a-provider)
           * [Upgrades providers without changing contract](#upgrades-providers-without-changing-contract)
           * [Upgrades providers and changing contract](#upgrades-providers-and-changing-contract)
           * [Changing a provider](#changing-a-provider)
           * [Deleting a provider](#deleting-a-provider)
        * [Upgrade from v1alpha3 management cluster to v1alpha4 cluster](#upgrade-from-v1alpha3-management-cluster-to-v1alpha4-cluster)
        * [Operator Lifecycle Management](#operator-lifecycle-management)
           * [Operator Installation](#operator-installation)
           * [Operator Upgrade](#operator-upgrade)
           * [Operator Delete](#operator-delete)
        * [Air gapped environment](#air-gapped-environment)
     * [Risks and Mitigation](#risks-and-mitigation)
        * [Error Handling &amp; Logging](#error-handling--logging)
        * [Extensibility Options](#extensibility-options)
        * [Upgrade from v1alpha3 management cluster to v1alpha4/operator cluster](#upgrade-from-v1alpha3-management-cluster-to-v1alpha4operator-cluster)
  * [Additional Details](#additional-details)
     * [Test Plan](#test-plan)
     * [Version Skew Strategy](#version-skew-strategy)
  * [Implementation History](#implementation-history)
  * [Controller Runtime Types](#controller-runtime-types)

## Glossary

The lexicon used in this document is described in more detail
[here](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/book/src/reference/glossary.md).
Any discrepancies should be rectified in the main Cluster API glossary.

## Summary

The clusterctl CLI currently handles the lifecycle of Cluster API
providers installed in a management cluster. It provides a great Day 0 and Day
1 experience in getting CAPI up and running. However, clusterctl’s imperative
design makes it difficult for cluster admins to stand up and manage CAPI
management clusters in their own preferred way.

This proposal provides a solution that leverages a declarative API and an
operator to empower admins to handle the lifecycle of providers within the
management cluster.

The operator is developed in a separate repository [TBD] and will have its own release cycle.

## Motivation

In its current form clusterctl is designed to provide a simple user experience
for day 1 operations of a Cluster API management cluster.

However such design is not optimized for supporting declarative approaches
when operating Cluster API management clusters.

These declarative approaches are important to enable GitOps workflows in case
users don't want to rely solely on the `clusterctl` CLI.

Providing a declarative API also enables us to leverage controller-runtime's
new component config and allow us to configure the controller manager and even
the resource limits of the provider's deployment.

Another example is improving cluster upgrades. In order to upgrade a cluster
we now need to supply all the information that was provided initially during a
`clusterctl init` which is inconvenient in many cases such as distributed
teams and CI pipelines where the configuration needs to be stored and synced
externally.

With the management cluster operator, we aim to address these use cases by
introducing an operator that handles the lifecycle of providers within the
management cluster based on a declarative API.

### Goals

- Define an API that enables declarative management of the lifecycle of
  Cluster API and all of its providers.
- Support air-gapped environments through sufficient documentation initially.
- Identify and document differences between clusterctl CLI and the operator in
  managing the lifecycle of providers, if any.
- Define how the clusterctl CLI should be changed in order to interact with
  the management cluster operator in a transparent and effective way.
- To support the ability to upgrade from a v1alpha3 based version (v0.3.[TBD])
  of Cluster API to one managed by the operator.

### Non-Goals/Future Work

- `clusterctl` related changes will be implemented after core operator functionality
  is complete. For example, deprecating `Provider` type and migrating to new ones.
- `clusterctl` will not be deprecated or replaced with another CLI.
- Implement an operator driven version of `clusterctl move`.
- Manage cert-manager using the operator.
- Support multiple installations of the same provider within a management
  cluster in light of [issue 3042] and [issue 3354].
- Support any template processing engines.
- Support the installation of v1alpha3 providers using the operator.

## Proposal

### User Stories

1. As an admin, I want to use a declarative style API to operate the Cluster
   API providers in a management cluster.
1. As an admin, I would like to have an easy and declarative way to change
   controller settings (e.g. enabling pprof for debugging).
1. As an admin, I would like to have an easy and declarative way to change the
   resource requirements (e.g. such as limits and requests for a provider
   deployment).
1. As an admin, I would like to have the option to use clusterctl CLI as of
   today, without being concerned about the operator.
1. As an admin, I would like to be able to install the operator using kubectl
   apply, without being forced to use clusterctl.

### Implementation Details/Notes/Constraints

### Clusterctl

The `clusterctl` CLI will provide a similar UX to the users whilst leveraging
the operator for the functions it can. As stated in the Goals/Non-Goals, the
move operation will not be driven by the operator but rather remain within the
CLI for now. However, this is an implementation detail and will not affect the
users. The move operation and all other `clusterctl` refactoring will be
done after core operator functionality is implemented.

#### Existing API Types Changes

The existing `Provider` type used by the clusterctl CLI will be deprecated and
its instances will be migrated to instances of the new API types as defined in
the next section.

The management cluster operator will be responsible for migrating the existing
provider types to support GitOps workflows excluding `clusterctl`.

#### New API Types

These are the new API types being defined.

There are separate types for each provider type - Core, Bootstrap,
ControlPlane, and Infrastructure. However, since each type is similar, their
Spec and Status uses the shared types - `ProviderSpec`,  `ProviderStatus`
respectively.

We will scope the CRDs to be namespaced. This will allow us to enforce
RBAC restrictions if needed. This also allows us to install multiple
versions of the controllers (grouped within namespaces) in the same
management cluster although this scenario will not be supported natively in
the v1alpha4 iteration.

If you prefer to see how the API can be used instead of reading the type
definition feel free to jump to the [Example API Usage
section](#example-api-usage)

```golang
// CoreProvider is the Schema for the CoreProviders API
type CoreProvider struct {
   metav1.TypeMeta   `json:",inline"`
   metav1.ObjectMeta `json:"metadata,omitempty"`

   Spec   ProviderSpec   `json:"spec,omitempty"`
   Status ProviderStatus `json:"status,omitempty"`
}

// BootstrapProvider is the Schema for the BootstrapProviders API
type BootstrapProvider struct {
   metav1.TypeMeta   `json:",inline"`
   metav1.ObjectMeta `json:"metadata,omitempty"`

   Spec   ProviderSpec   `json:"spec,omitempty"`
   Status ProviderStatus `json:"status,omitempty"`
}

// ControlPlaneProvider is the Schema for the ControlPlaneProviders API
type ControlPlaneProvider struct {
   metav1.TypeMeta   `json:",inline"`
   metav1.ObjectMeta `json:"metadata,omitempty"`

   Spec   ProviderSpec   `json:"spec,omitempty"`
   Status ProviderStatus `json:"status,omitempty"`
}

// InfrastructureProvider is the Schema for the InfrastructureProviders API
type InfrastructureProvider struct {
   metav1.TypeMeta   `json:",inline"`
   metav1.ObjectMeta `json:"metadata,omitempty"`

   Spec   ProviderSpec   `json:"spec,omitempty"`
   Status ProviderStatus `json:"status,omitempty"`
}
```

Below you can find details about `ProviderSpec`,  `ProviderStatus`, which is
shared among all the provider types - Core, Bootstrap, ControlPlane, and
Infrastructure.

```golang
// ProviderSpec defines the desired state of the Provider.
type ProviderSpec struct {
   // Version indicates the provider version.
   // +optional
   Version *string `json:"version,omitempty"`

   // Manager defines the properties that can be enabled on the controller manager for the provider.
   // +optional
   Manager ManagerSpec `json:"manager,omitempty"`

   // Deployment defines the properties that can be enabled on the deployment for the provider.
   // +optional
   Deployment *DeploymentSpec `json:"deployment,omitempty"`

   // SecretName is the name of the Secret providing the configuration
   // variables for the current provider instance, like e.g. credentials.
   // Such configurations will be used when creating or upgrading provider components.
   // The contents of the secret will be treated as immutable. If changes need
   // to be made, a new object can be created and the name should be updated.
   // The contents should be in the form of key:value. This secret must be in
   // the same namespace as the provider.
   // +optional
   SecretName *string `json:"secretName,omitempty"`

   // FetchConfig determines how the operator will fetch the components and metadata for the provider.
   // If nil, the operator will try to fetch components according to default
   // embedded fetch configuration for the given kind and `ObjectMeta.Name`.
   // For example, the infrastructure name `aws` will fetch artifacts from
   // https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases.
   // +optional
   FetchConfig *FetchConfiguration `json:"fetchConfig,omitempty"`

   // Paused prevents the operator from reconciling the provider. This can be
   // used when doing an upgrade or move action manually.
   // +optional
   Paused bool `json:"paused,omitempty"`
}

// ManagerSpec defines the properties that can be enabled on the controller manager for the provider.
type ManagerSpec struct {
   // ControllerManagerConfigurationSpec defines the desired state of GenericControllerManagerConfiguration.
   ctrlruntime.ControllerManagerConfigurationSpec `json:",inline"`

   // ProfilerAddress defines the bind address to expose the pprof profiler (e.g. localhost:6060).
   // Default empty, meaning the profiler is disabled.
   // Controller Manager flag is --profiler-address.
   // +optional
   ProfilerAddress *string `json:"profilerAddress,omitempty"`

   // MaxConcurrentReconciles is the maximum number of concurrent Reconciles
   // which can be run. Defaults to 10.
   // +optional
   MaxConcurrentReconciles *int `json:"maxConcurrentReconciles,omitempty"`

   // Verbosity set the logs verbosity. Defaults to 1.
   // Controller Manager flag is --verbosity.
   // +optional
   Verbosity int `json:"verbosity,omitempty"`

   // Debug, if set, will override a set of fields with opinionated values for
   // a debugging session. (Verbosity=5, ProfilerAddress=localhost:6060)
   // +optional
   Debug bool `json:"debug,omitempty"`

   // FeatureGates define provider specific feature flags that will be passed
   // in as container args to the provider's controller manager.
   // Controller Manager flag is --feature-gates.
   FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// DeploymentSpec defines the properties that can be enabled on the Deployment for the provider.
type DeploymentSpec struct {
   // Number of desired pods. This is a pointer to distinguish between explicit zero and not specified. Defaults to 1.
   // +optional
   Replicas *int `json:"replicas,omitempty"`

   // NodeSelector is a selector which must be true for the pod to fit on a node.
   // Selector which must match a node's labels for the pod to be scheduled on that node.
   // More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
   // +optional
   NodeSelector map[string]string `json:"nodeSelector,omitempty"`

   // If specified, the pod's tolerations.
   // +optional
   Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

   // If specified, the pod's scheduling constraints
   // +optional
   Affinity *corev1.Affinity `json:"affinity,omitempty"`

   // List of containers specified in the Deployment
   // +optional
   Containers []ContainerSpec `json:"containers"`
}

// ContainerSpec defines the properties available to override for each
// container in a provider deployment such as Image and Args to the container’s
// entrypoint.
type ContainerSpec struct {
   // Name of the container. Cannot be updated.
   Name string `json:"name"`

   // Container Image Name
   // +optional
   Image *ImageMeta `json:"image,omitempty"`

   // Args represents extra provider specific flags that are not encoded as fields in this API.
   // Explicit controller manager properties defined in the `Provider.ManagerSpec`
   // will have higher precedence than those defined in `ContainerSpec.Args`.
   // For example, `ManagerSpec.SyncPeriod` will be used instead of the
   // container arg `--sync-period` if both are defined.
   // The same holds for `ManagerSpec.FeatureGates` and `--feature-gates`.
   // +optional
   Args map[string]string `json:"args,omitempty"`

   // List of environment variables to set in the container.
   // +optional
   Env []corev1.EnvVar `json:"env,omitempty"`

   // Compute resources required by this container.
   // +optional
   Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ImageMeta allows to customize the image used
type ImageMeta struct {
    // Repository sets the container registry to pull images from.
    // +optional
    Repository *string `json:"repository,omitempty`

    // Name allows to specify a name for the image.
    // +optional
    Name *string `json:"name,omitempty`

    // Tag allows to specify a tag for the image.
    // +optional
    Tag *string `json:"tag,omitempty`
}

// FetchConfiguration determines the way to fetch the components and metadata for the provider.
type FetchConfiguration struct {
   // URL to be used for fetching the provider’s components and metadata from a remote Github repository.
   // For example, https://github.com/{owner}/{repository}/releases
   // The version of the release will be `ProviderSpec.Version` if defined
   // otherwise the `latest` version will be computed and used.
   // +optional
   URL *string `json:"url,omitempty"`

   // Selector to be used for fetching provider’s components and metadata from
   // ConfigMaps stored inside the cluster. Each ConfigMap is expected to contain
   // components and metadata for a specific version only.
   // +optional
   Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// ProviderStatus defines the observed state of the Provider.
type ProviderStatus struct {
   // Contract will contain the core provider contract that the provider is
   // abiding by, like e.g. v1alpha3.
   // +optional
   Contract *string `json:"contract,omitempty"`

   // Conditions define the current service state of the cluster.
   // +optional
   Conditions Conditions `json:"conditions,omitempty"`

   // ObservedGeneration is the latest generation observed by the controller.
   // +optional
   ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
```

**Validation and defaulting rules for Provider and ProviderSpec**
- The `Name` field within `metav1.ObjectMeta` could be any valid Kubernetes
  name; however, it is recommended to use Cluster API provider names. For
  example, aws, vsphere, kubeadm. These names will be used to fetch the
  default configurations in case there is no specific FetchConfiguration
  defined.
- `ProviderSpec.Version` should be a valid default version with the "v" prefix
  as commonly used in the Kubernetes ecosystem; if this value is nil when a
  new provider is created, the operator will determine the version to use
  applying the same rules implemented in clusterctl (latest).
  Once the latest version is calculated it will be set in
  `ProviderSpec.Version`.
- Note: As per discussion in the CAEP PR, we will keep the `SecretName` field
  to allow the provider authors ample time to implement their own credential
  management to support multiple workload clusters. [See this thread for more
  info][secret-name-discussion].

**Validation rules for ProviderSpec.FetchConfiguration**
- If the FetchConfiguration is empty and not defined, then the operator will
  apply the embedded fetch configuration for the given kind and
  `ObjectMeta.Name`. For example, the infrastructure name `aws` will fetch
  artifacts from
  https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases.
- If FetchConfiguration is not nil, exactly one of `URL` or `Selector` must be
  specified.
- `FetchConfiguration.Selector` is used to fetch provider’s components and
  metadata from ConfigMaps stored inside the cluster. Each ConfigMap is
  expected to contain components and metadata for a specific version only. So
  if multiple versions of the providers need to be specified, they can be
  added as separate ConfigMaps and labeled with the same selector. This
  provides the same behavior as the “local” provider repositories but now from
  within the management cluster.
- `FetchConfiguration` is used only during init and upgrade operations.
  Changes made to the contents of `FetchConfiguration` will not trigger a
  reconciliation. This is similar behavior to `ProviderSpec.SecretName`.

**Validation Rules for ProviderSpec.ManagerSpec**
- The ControllerManagerConfigurationSpec is a type from
  `controller-runtime/pkg/config` and is an embedded into the `ManagerSpec`.
  This type will expose LeaderElection, SyncPeriod, Webhook, Health and
  Metrics configurations.
- If `ManagerSpec.Debug` is set to true, the operator will not allow changes
  to other properties since it is in Debug mode.
- If you need to set specific concurrency values for each reconcile loop (e.g.
  `awscluster-concurrency`), you can leave
  `ManagerSpec.MaxConcurrentReconciles` nil and use `Container.Args`.
- If `ManagerSpec.MaxConcurrentReconciles` is set and a specific concurrency
  flag such as `awscluster-concurrency` is set on the `Container.Args`, then
  the more specific concurrency flag will have higher precedence.


**Validation Rules for ContainerSpec**
- The `ContainerSpec.Args` will ignore the key `namespace` since the operator
  enforces a deployment model where all the providers should be configured to
  watch all the namespaces.
- Explicit controller manager properties defined in the `Provider.ManagerSpec`
  will have higher precedence than those defined in `ContainerSpec.Args`. That
  is, if `ManagerSpec.SyncPeriod` is defined it will be used instead of the
  container arg `sync-period`. This is true also for
  `ManagerSpec.FeatureGates`, that is, it will have higher precedence to the
  container arg `feature-gates`.
- If no `ContainerSpec.Resources` are defined, the defaults on the Deployment
  object within the provider’s components yaml will be used.


#### Example API Usage

1. As an admin, I want to install the aws infrastructure provider with
   specific controller flags.

```yaml
apiVersion: v1
kind: Secret
metadata:
 name: aws-variables
 namespace: capa-system
type: Opaque
data:
 AWS_REGION: ...
 AWS_ACCESS_KEY_ID: ...
 AWS_SECRET_ACCESS_KEY: ...
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: InfrastructureProvider
metadata:
 name: aws
 namespace: capa-system
spec:
 version: v0.6.0
 secretName: aws-variables
 manager:
   # These top level controller manager flags, supported by all the providers.
   # These flags come with sensible defaults, thus requiring no or minimal
   # changes for the most common scenarios.
   metricsAddress: ":8181"
   syncPeriod: 660
 fetchConfig:
   url: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases
 deployment:
   containers:
   - name: manager
     args:
         # These are controller flags that are specific to a provider; usage
         # is reserved for advanced scenarios only.
         awscluster-concurrency: 12
         awsmachine-concurrency: 11
```

2. As an admin, I want to install aws infrastructure provider but override
   the container image of the CAPA deployment.

```yaml
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: InfrastructureProvider
metadata:
 name: aws
 namespace: capa-system
spec:
 version: v0.6.0
 secretName: aws-variables
 deployment:
   containers:
   - name: manager
     image: gcr.io/myregistry/capa-controller:v0.6.0-foo
```

3. As an admin, I want to change the resource limits for the manager pod in
   my control plane provider deployment.

```yaml
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: ControlPlaneProvider
metadata:
 name: kubeadm
 namespace: capi-kubeadm-control-plane-system
spec:
 version: v0.3.10
 secretName: capi-variables
 deployment:
   containers:
   - name: manager
     resources:
       limits:
         cpu: 100m
         memory: 30Mi
       requests:
         cpu: 100m
         memory: 20Mi
```

4. As an admin, I would like to fetch my azure provider components from a
   specific repository which is not the default.

```yaml
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: InfrastructureProvider
metadata:
 name: myazure
 namespace: capz-system
spec:
 version: v0.4.9
 secretName: azure-variables
 fetchConfig:
   url: https://github.com/myorg/awesome-azure-provider/releases

```

5. As an admin, I would like to use the default fetch configurations by
   simply specifying the expected Cluster API provider names such as 'aws',
   'vsphere', 'azure', 'kubeadm', 'talos', or 'cluster-api' instead of having
   to explicitly specify the fetch configuration.
   In the example below, since we are using 'vsphere' as the name of the
   InfrastructureProvider the operator will fetch it's configuration from
   `url: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases`
   by default.

See more examples in the [air-gapped environment section](#air-gapped-environment)

```yaml
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: InfrastructureProvider
metadata:
 name: vsphere
 namespace: capv-system
spec:
 version: v0.4.9
 secretName: vsphere-variables

```

#### Operator Behaviors

##### Installing a provider

In order to install a new Cluster API provider with the management cluster
operator you have to create a provider as shown above. See the first example
API usage to create the secret with variables and the provider itself.

When processing a Provider object the operator will apply the following rules.

- Providers with  `spec.Type == CoreProvider` will be installed first; the
  other providers will be requeued until the core provider exists.
- Before installing any provider following preflight checks will be executed :
    - There should not be another instance of the same provider (same Kind, same
      name) in any namespace.
    - The Cluster API contract the provider is abiding by, e.g. v1alpha4, must
      match the contract of the core provider.
- The operator will set conditions on the Provider object to surface any
  installation issues such as pre-flight checks and/or order of installation
  to accurately inform the user.
- Since the FetchConfiguration is empty and not defined, the operator will
  apply the embedded fetch configuration for the given kind and
  `ObjectMeta.Name`. In this case, the operator will fetch artifacts from
  https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases.

The installation process managed by the operator is consistent with the
implementation underlying the `clusterctl init` command and includes the
following steps:
- Fetching the provider artifacts (the components yaml and the metadata.yaml
  file).
- Applying image overrides, if any.
- Replacing variables in the infrastructure-components from EnvVar and
  Secret.
- Applying the resulting yaml to the cluster.

As a final consideration, please note that
- The operator executes installation for 1 provider at time, while `clusterctl
  init` manages installation of a group of providers with a single operation.
- `clusterctl init` uses environment variables and a local configuration file,
  while the operator uses a Secret; given that we want the users to preserve
  current behaviour in clusterctl, the init operation should be modified to
  transfer local configuration to the cluster.
  As part of `clusterctl init`, it will obtain the list of variables required
  by the provider components and read the corresponding values from the config
  or environment variables and build the secret.
  Any image overrides defined in the clusterctl config will also be applied to
  the provider's components.

In the following figure, the controllers for the providers are installed in
the namespaces that are defined by default.

![Figure 1](./images/capi-provider-operator/fig3.png "Figure for
installing providers in defined namespaces")
<div align="center">Installing providers in defined namespaces</div>
<br/>

In the following figure, the controllers for the providers are all installed in
the same namespace as configured by the user.

![Figure 2](./images/capi-provider-operator/fig4.png "Figure for
installing all providers in the same namespace")
<div align="center">Installing all providers in the same namespace</div>
<br/>

##### Upgrading a provider

In order to trigger an upgrade of a new Cluster API provider you have to
change the `spec.Version` field.

Upgrading a provider in the management cluster must abide by the golden rule
that all the providers should respect the same Cluster API contract supported
by the core provider.

##### Upgrades providers without changing contract

If the new version of the provider does abide by the same version of the
Cluster API contract, the operator will execute the upgrade by performing:
- Delete of the current instance of the provider components, while preserving
  CRDs, namespace and user objects.
- Install the new version of the provider components

Please note that:
- The operator executes upgrades 1 provider at time, while `clusterctl upgrade
  apply` manages upgrading a group of providers with a single operation.
- `clusterctl upgrade apply --contract` automatically determines the latest
  versions available for each provider, while with the Declarative approach
  the user is responsible for manually editing Provider objects yaml.
- `clusterctl upgrade apply` currently uses environment variables and a local
  configuration file; this should be changed in order to use in cluster
  provider configurations.

![Figure 3](./images/capi-provider-operator/fig1.png "Figure for
upgrading provider without changing contract")
<div align="center">Upgrading providers without changing contract</div>
<br/>

##### Upgrades providers and changing contract

If the new version of the provider does abide by a new version of the Cluster
API contract, it is required to ensure all the other providers in the
management cluster should get the new version too.

![Figure 4](./images/capi-provider-operator/fig2.png "Figure for
upgrading provider and changing contract")
<div align="center">Upgrading providers and changing contract</div>
<br/>

As a first step, it is required to pause all the providers by setting the
`spec.Paused` field to true for each provider; the operator will block any
contract upgrade until all the providers are paused.

After all the providers are in paused state, you can proceed with the upgrade
as described in the previous paragraph (change the `spec.Version` field).

When a provider is paused the number of replicas will be scaled to 0; the
operator will add a new
`management.cluster.x-k8s.io/original-controller-replicas` annotation to store
the original replica count.

Once all the providers are upgraded to a version that abides to the new
contract, it is possible for the operator to unpause providers; the operator
does not allow to unpause providers if there are still providers abiding to
the old contract.

Please note that we are planning to embed this sequence (pause - upgrade -
unpause) as a part of `clusterctl upgrade apply` command when there is a
contract change.

##### Changing a provider

On top of changing a provider version (upgrades), the operator supports also
changing other provider fields, most notably controller flags and variables.
This can be achieved by either `kubectl edit` or `kubectl apply` to the
provider object.

The operation internally works like upgrades: The current instance of the
provider is deleted, while preserving CRDs, namespaced and user objects A new
instance of the provider is installed with the new set of flags/variables.

Please note that clusterctl currently does not support this operation.

See Example 1 in [Example API Usage](#example-api-usage)

##### Deleting a provider

In order to delete a provider you have to delete the corresponding provider
object.

Deletion of the provider will be blocked if any workload cluster using the
provider still exists.

Additionally, deletion of a core provider should be blocked if there are still
other providers in the management cluster.

#### Upgrade from v1alpha3 management cluster to v1alpha4 cluster

Cluster API will provide instructions on how to upgrade from a v1alpha3
management cluster, created by clusterctl to the new v1alpha4 management
cluster. These operations could require manual actions.

Some of the actions are described below:
- Run webhooks as part of the main manager. See [issue 3822].

More details will be added as we better understand what a v1alpha4 cluster
will look like.

#### Operator Lifecycle Management

##### Operator Installation

- During the first phase of implementation `clusterctl` won't provide support
  for managing the operator, so the admin will have to install it manually using 
  `kubectl apply` (or similar solutions), the operator yaml that will be published in the
  operator subproject release artifacts.
- In future `clusterctl init` will install the operator and its corresponding CRDs 
  as a pre-requisite if the operator doesn’t already exist. Please note that this
  command will consider image overrides defined in the local clusterctl config
  file.

##### Operator Upgrade
- During the first phase of implementation `clusterctl` operations will not be 
  supported and admin will have to install the operator manually, or in case if
  the admin doesn’t want to use clusterctl, they can use `kubectl apply`(or similar solutions) 
  with the latest version of the operator yaml that will be published in the
  operator subproject release artifacts.
- The transition between manually managed operator and clusterctl managed
  operator will be documented later as we progress with the implementation.
- In future the admin will be able to use `clusterctl upgrade operator` to 
  upgrade the operator components. Please note that this command will consider 
  image overrides defined in the local clusterctl config file. Other commands 
  such as `clusterctl upgrade apply` will also allow to upgrade the operator.
- `clusterctl upgrade plan` will identify when the operator can be upgraded by
  checking the cluster-api release artifacts.
- clusterctl will require a matching operator version. In the future, when
  clusterctl move to beta/GA, we will reconsider supporting version skew
  between clusterctl and the operator.

##### Operator Delete
- During the first phase of implementation `clusterctl` operations will not be 
  supported and admin will have to delete the operator manually using `kubectl delete`
  (or similar solutions). However, it’s the admin’s responsibility to verify that there 
  are no providers running in the management cluster.
- In future the clusterctl will delete the operator as part of 
  the `clusterctl delete --all` command.

#### Air gapped environment

In order to install Cluster API providers in an air-gapped environment using
the operator, it is required to address the following issues.

1. Make the operator work in air-gapped environment
   - To provide image overrides for the operator itself in order to pull the
     images from an accessible image repository. Please note that the
     overrides will be considered from the image overrides defined in the
     local clusterctl config file.
   - TBD if operator yaml will be embedded in clusterctl or if it should be a
     special artifact within the core provider repository.
1. Make the providers work in air-gapped environment
   - To provide fetch configuration for each provider reading from an
     accessible location (e.g. an internal github repository) or from
     ConfigMaps pre-created inside the cluster.
   - To provide image overrides for each provider in order to pull the images
     from an accessible image repository.

**Example Usage**

As an admin, I would like to fetch my azure provider components from within
the cluster because I’m working within an air-gapped environment.

In this example, we have two config maps that define the components and
metadata of the provider. They each share the label `provider-components:
azure` and are within the `capz-system` namespace.

The azure InfrastructureProvider has a `fetchConfig` which specifies the label
selector. This way the operator knows which versions of the azure provider are
available. Since the provider’s version is marked as `v0.4.9`, it uses the
components information from the config map to install the azure provider.

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
 labels:
   provider-components: azure
 name: v0.4.9
 namespace: capz-system
data:
 components: |
 # components for v0.4.9 yaml goes here
 metadata: |
 # metadata information goes here
---
apiVersion: v1
kind: ConfigMap
metadata:
 labels:
   provider-components: azure
 name: v0.4.8
 namespace: capz-system
data:
 components: |
 # components for v0.4.8 yaml goes here
 metadata: |
 # metadata information goes here
---
apiVersion: management.cluster.x-k8s.io/v1alpha1
kind: InfrastructureProvider
metadata:
 name: azure
 namespace: capz-system
spec:
 version: v0.4.9
 secretName: azure-variables
 fetchConfig:
   selector:
     matchLabels:
       provider-components: azure
```

### Risks and Mitigation

#### Error Handling & Logging

Currently, clusterctl provides quick feedback regarding required variables
etc. With the operator in place we’ll need to ensure that the error messages
and logs are easily available to the user to verify progress.

#### Extensibility Options

Currently, clusterctl has a few extensibility options.  For example,
clusterctl is built on-top of a library that can be leveraged to build other
tools.

It also exposes an interface for template processing if we choose to go a
different route from `envsubst`. This may prove to be challenging in the
context of the operator as this would mean a change to the operator
binary/image. We could introduce a new behavior or communication protocol or
hooks for the operator to interact with the custom template processor. This
could be configured similarly to the fetch config, with multiple options built
in.

We have decided that supporting multiple template processors is a non-goal for
this implementation of the proposal and we will rely on using the default
`envsubst` template processor.

#### Upgrade from v1alpha3 management cluster to v1alpha4/operator cluster

As of today, this is hard to define as have yet to understand the definition
of what a v1alpha4 cluster will be. Once we better understand what a v1alpha4
cluster will look like, we will then be able to determine the upgrade sequence
from v1alpha3.

Cluster API will provide instructions on how to upgrade from a v1alpha3
management cluster, created by clusterctl to the new v1alpha4 management
cluster. These operations could require manual actions.

Some of the actions are described below:
- Run webhooks as part of the main manager. See [issue
  3822](https://github.com/kubernetes-sigs/cluster-api/issues/3822)

## Additional Details

### Test Plan

The operator will be written with unit and integration tests using envtest and
existing patterns as defined under the [Developer
Guide/Testing](https://cluster-api.sigs.k8s.io/developer/testing.html) section
in the Cluster API book.

Existing E2E tests will verify that existing clusterctl commands such as `init`
and `upgrade` will work as expected. Any necessary changes will be made in
order to make it configurable.

New E2E tests verifying the operator lifecycle itself will be added.

New E2E tests verifying the upgrade from a v1alpha3 to v1alpha4 cluster will
be added.

### Version Skew Strategy

- clusterctl will require a matching operator version. In the future, when
  clusterctl move to beta/GA, we will reconsider supporting version skew
  between clusterctl and the operator.

## Implementation History

- [x] 09/09/2020: Proposed idea in an issue or [community meeting]
- [x] 09/14/2020: Compile a [Google Doc following the CAEP template][management cluster operator caep]
- [x] 09/14/2020: First round of feedback from community
- [x] 10/07/2020: Present proposal at a [community meeting]
- [ ] 10/20/2020: Open proposal PR

## Controller Runtime Types

These types are pulled from [controller-runtime][controller-runtime-code-ref]
and [component-base][components-base-code-ref]. They are used as part of the
`ManagerSpec`. They are duplicated here for convenience sake.

```golang
// ControllerManagerConfigurationSpec defines the desired state of GenericControllerManagerConfiguration
type ControllerManagerConfigurationSpec struct {
	// SyncPeriod determines the minimum frequency at which watched resources are
	// reconciled. A lower period will correct entropy more quickly, but reduce
	// responsiveness to change if there are many watched resources. Change this
	// value only if you know what you are doing. Defaults to 10 hours if unset.
	// there will a 10 percent jitter between the SyncPeriod of all controllers
	// so that all controllers will not send list requests simultaneously.
	// +optional
	SyncPeriod *metav1.Duration `json:"syncPeriod,omitempty"`

	// LeaderElection is the LeaderElection config to be used when configuring
	// the manager.Manager leader election
	// +optional
	LeaderElection *configv1alpha1.LeaderElectionConfiguration `json:"leaderElection,omitempty"`

	// CacheNamespace if specified restricts the manager's cache to watch objects in
	// the desired namespace Defaults to all namespaces
	//
	// Note: If a namespace is specified, controllers can still Watch for a
	// cluster-scoped resource (e.g Node).  For namespaced resources the cache
	// will only hold objects from the desired namespace.
	// +optional
	CacheNamespace string `json:"cacheNamespace,omitempty"`

	// GracefulShutdownTimeout is the duration given to runnable to stop before the manager actually returns on stop.
	// To disable graceful shutdown, set to time.Duration(0)
	// To use graceful shutdown without timeout, set to a negative duration, e.G. time.Duration(-1)
	// The graceful shutdown is skipped for safety reasons in case the leader election lease is lost.
	GracefulShutdownTimeout *metav1.Duration `json:"gracefulShutDown,omitempty"`

	// Metrics contains thw controller metrics configuration
	// +optional
	Metrics ControllerMetrics `json:"metrics,omitempty"`

	// Health contains the controller health configuration
	// +optional
	Health ControllerHealth `json:"health,omitempty"`

	// Webhook contains the controllers webhook configuration
	// +optional
	Webhook ControllerWebhook `json:"webhook,omitempty"`
}

// ControllerMetrics defines the metrics configs
type ControllerMetrics struct {
	// BindAddress is the TCP address that the controller should bind to
	// for serving prometheus metrics.
	// It can be set to "0" to disable the metrics serving.
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// ControllerHealth defines the health configs
type ControllerHealth struct {
	// HealthProbeBindAddress is the TCP address that the controller should bind to
	// for serving health probes
	// +optional
	HealthProbeBindAddress string `json:"healthProbeBindAddress,omitempty"`

	// ReadinessEndpointName, defaults to "readyz"
	// +optional
	ReadinessEndpointName string `json:"readinessEndpointName,omitempty"`

	// LivenessEndpointName, defaults to "healthz"
	// +optional
	LivenessEndpointName string `json:"livenessEndpointName,omitempty"`
}

// ControllerWebhook defines the webhook server for the controller
type ControllerWebhook struct {
	// Port is the port that the webhook server serves at.
	// It is used to set webhook.Server.Port.
	// +optional
	Port *int `json:"port,omitempty"`

	// Host is the hostname that the webhook server binds to.
	// It is used to set webhook.Server.Host.
	// +optional
	Host string `json:"host,omitempty"`

	// CertDir is the directory that contains the server key and certificate.
	// if not set, webhook server would look up the server key and certificate in
	// {TempDir}/k8s-webhook-server/serving-certs. The server key and certificate
	// must be named tls.key and tls.crt, respectively.
	// +optional
	CertDir string `json:"certDir,omitempty"`
}

// LeaderElectionConfiguration defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionConfiguration struct {
	// leaderElect enables a leader election client to gain leadership
	// before executing the main loop. Enable this when running replicated
	// components for high availability.
	LeaderElect *bool `json:"leaderElect"`
	// leaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of a led but unrenewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration metav1.Duration `json:"leaseDuration"`
	// renewDeadline is the interval between attempts by the acting master to
	// renew a leadership slot before it stops leading. This must be less
	// than or equal to the lease duration. This is only applicable if leader
	// election is enabled.
	RenewDeadline metav1.Duration `json:"renewDeadline"`
	// retryPeriod is the duration the clients should wait between attempting
	// acquisition and renewal of a leadership. This is only applicable if
	// leader election is enabled.
	RetryPeriod metav1.Duration `json:"retryPeriod"`
	// resourceLock indicates the resource object type that will be used to lock
	// during leader election cycles.
	ResourceLock string `json:"resourceLock"`
	// resourceName indicates the name of resource object that will be used to lock
	// during leader election cycles.
	ResourceName string `json:"resourceName"`
	// resourceName indicates the namespace of resource object that will be used to lock
	// during leader election cycles.
	ResourceNamespace string `json:"resourceNamespace"`
}
```

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
[management cluster operator caep]: https://docs.google.com/document/d/1fQNlqsDkvEggWFi51GVxOglL2P1Bvo2JhZlMhm2d-Co/edit#
[controller-runtime-code-ref]: https://github.com/kubernetes-sigs/controller-runtime/blob/5c2b42d0dfe264fe1a187dcb11f384c0d193c042/pkg/config/v1alpha1/types.go
[components-base-code-ref]: https://github.com/kubernetes/component-base/blob/3b346c3e81285da5524c9379262ad4ca327b3c75/config/v1alpha1/types.go
[issue 3042]: https://github.com/kubernetes-sigs/cluster-api/issues/3042
[issue 3354]: https://github.com/kubernetes-sigs/cluster-api/issues/3354
[issue 3822]: https://github.com/kubernetes-sigs/cluster-api/issues/3822)
[secret-name-discussion]: https://github.com/kubernetes-sigs/cluster-api/pull/3833#discussion_r540576353
