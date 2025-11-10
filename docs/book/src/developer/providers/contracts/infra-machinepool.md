# Contract rules for InfraMachinePool

Infrastructure providers CAN OPTIONALLY implement an InfraMachinePool resource using Kubernetes' CustomResourceDefinition (CRD).

The goal of an InfraMachinePool is to manage the lifecycle of a provider-specific pool of machines using a provider specific service (like auto-scale groups in AWS & virtual machine scalesets in Azure).

The machines in the pool may be physical or virtual instances (although most likely virtual), and they represent the infrastructure for Kubernetes nodes.

The InfraMachinePool resource will be referenced by one of the Cluster API core resources, MachinePool.

The [MachinePool's controller](../../core/controllers/machine-pool.md) is responsible to coordinate operations of the InfraMachinePool, and the interaction between the MachinePool's controller and the InfraMachinePool is based on the contract rules defined in this page.

Once contract rules are satisfied by an InfraMachinePool implementation, other implementation details
could be addressed according to the specific needs (Cluster API is not prescriptive).

Nevertheless, it is always recommended to take a look at Cluster API controllers,
in-tree providers, other providers and use them as a reference implementation (unless custom solutions are required
in order to address very specific needs).

<aside class="note warning">

<h1>Never rely on Cluster API behaviours not defined as a contract rule!</h1>

When developing a provider, you MUST consider any Cluster API behaviour that is not defined by a contract rule
as a Cluster API internal implementation detail, and internal implementation details can change at any time.

Accordingly, in order to not expose users to the risk that your provider breaks when the Cluster API internal behavior
changes, you MUST NOT rely on any Cluster API internal behaviour when implementing an InfraMachinePool resource.

Instead, whenever you need something more from the Cluster API contract, you MUST engage the community.

The Cluster API maintainers welcome feedback and contributions to the contract in order to improve how it's defined,
its clarity and visibility to provider implementers and its suitability across the different kinds of Cluster API providers.

To provide feedback or open a discussion about the provider contract please [open an issue on the Cluster API](https://github.com/kubernetes-sigs/cluster-api/issues/new?assignees=&labels=&template=feature_request.md)
repo or add an item to the agenda in the [Cluster API community meeting](https://git.k8s.io/community/sig-cluster-lifecycle/README.md#cluster-api).

</aside>

## Rules (contract version v1beta2)

| Rule                                                                 | Mandatory | Note                                 |
|----------------------------------------------------------------------|-----------|--------------------------------------|
| [All resources: scope]                                               | Yes       |                                      |
| [All resources: `TypeMeta` and `ObjectMeta`field]                    | Yes       |                                      |
| [All resources: `APIVersion` field value]                            | Yes       |                                      |
| [InfraMachinePool, InfraMachinePoolList resource definition]         | Yes       |                                      |
| [InfraMachinePool: instances]                                        | No        |                                      |
| [MachinePoolMachines support]                                        | No        |                                      |
| [InfraMachinePool: providerID]                                       | No        |                                      |
| [InfraMachinePool: providerIDList]                                   | Yes       |                                      |
| [InfraMachinePool: initialization completed]                         | Yes       |                                      |
| [InfraMachinePool: pausing]                                          | No        |                                      |
| [InfraMachinePool: conditions]                                       | No        |                                      |
| [InfraMachinePool: replicas]                                         | Yes       |                                      |
| [InfraMachinePool: terminal failures]                                | No        |                                      |
| [InfraMachinePoolTemplate, InfraMachineTemplatePoolList resource definition] | No | Mandatory for ClusterClasses support |
| [InfraMachinePoolTemplate: support for SSA dry run]                  | No        | Mandatory for ClusterClasses support |
| [Multi tenancy]                                                      | No        | Mandatory for clusterctl CLI support |
| [Clusterctl support]                                                 | No        | Mandatory for clusterctl CLI support |

Note:

- `All resources` refers to all the provider's resources "core" Cluster API interacts with;
  In the context of this page: `InfraMachinePool`, `InfraMachinePoolTemplate` and corresponding list types
  
### All resources: scope

All resources MUST be namespace-scoped.

### All resources: `TypeMeta` and `ObjectMeta` field

All resources MUST have the standard Kubernetes `TypeMeta` and `ObjectMeta` fields.

### All resources: `APIVersion` field value

In Kubernetes `APIVersion` is a combination of API group and version.
Special consideration MUST applies to both API group and version for all the resources Cluster API interacts with.

#### All resources: API group

The domain for Cluster API resources is `cluster.x-k8s.io`, and infrastructure providers under the Kubernetes SIGS org
generally use `infrastructure.cluster.x-k8s.io` as API group.

If your provider uses a different API group, you MUST grant full read/write RBAC permissions for resources in your API group
to the Cluster API core controllers. The canonical way to do so is via a `ClusterRole` resource with the [aggregation label]
`cluster.x-k8s.io/aggregate-to-manager: "true"`.

The following is an example ClusterRole for a `FooMachinePool` resource in the `infrastructure.foo.com` API group:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: capi-foo-clusters
    labels:
      cluster.x-k8s.io/aggregate-to-manager: "true"
rules:
- apiGroups:
    - infrastructure.foo.com
  resources:
    - foomachinepools
    - foomachinepooltemplates
  verbs:
    - create
    - delete
    - get
    - list
    - patch
    - update
    - watch
```

Note: The write permissions are required because Cluster API manages InfraMachinePools generated from InfraMachinePoolTemplates; when using ClusterClass and managed topologies, also InfraMachinePoolTemplates are managed directly by Cluster API.

#### All resources: version

The resource Version defines the stability of the API and its backward compatibility guarantees.
Examples include `v1alpha1`, `v1beta1`, `v1`, etc. and are governed by the [Kubernetes API Deprecation Policy].

Your provider SHOULD abide by the same policies.

Note: The version of your provider does not need to be in sync with the version of core Cluster API resources.
Instead, prefer choosing a version that matches the stability of the provider API and its backward compatibility guarantees.

Additionally:

Providers MUST set `cluster.x-k8s.io/<version>` label on the InfraMachinePool Custom Resource Definitions.

The label is a map from a Cluster API contract version to your Custom Resource Definition versions.
The value is an underscore-delimited (_) list of versions. Each value MUST point to an available version in your CRD Spec.

The label allows Cluster API controllers to perform automatic conversions for object references, the controllers will pick
the last available version in the list if multiple versions are found.

To apply the label to CRDs it’s possible to use labels in your `kustomization.yaml` file, usually in `config/crd`:

```yaml
labels:
- pairs:
    cluster.x-k8s.io/v1beta1: v1beta1
    cluster.x-k8s.io/v1beta2: v1beta2
```

An example of this is in the [AWS infrastructure provider](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/config/crd/kustomization.yaml).

### InfraMachinePool, InfraMachinePoolList resource definition

You MUST define a InfraMachinePool resource if you provider supports MachinePools.
The InfraMachinePool resource name must have the format produced by `sigs.k8s.io/cluster-api/util/contract.CalculateCRDName(Group, Kind)`.

Note: Cluster API is using such a naming convention to avoid an expensive CRD lookup operation when looking for labels from
the CRD definition of the InfraMachinePool resource.

It is a generally applied convention to use names in the format `${env}MachinePool`, where ${env} is a, possibly short, name
for the environment in question. For example `AWSMachinePool` is an implementation for Amazon Web Services, and `AzureMachinePool`
is one for Azure.

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foomachinepools,shortName=foomp,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of FooMachinePool"

// FooMachinePool is the Schema for foomachinepools.
type FooMachinePool struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec FooMachinePoolSpec `json:"spec,omitempty"`
    Status FooMachinePoolStatus `json:"status,omitempty"`
}

type FooMachinePoolSpec struct {
    // See other rules for more details about mandatory/optional fields in InfraMachinePool spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

type FooMachinePoolStatus struct {
    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

For each InfraMachinePool resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<InfraMachinePool>List`.

```go
// +kubebuilder:object:root=true

// FooMachinePoolList contains a list of foomachinepools.
type FooMachinePoolList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooMachinePool `json:"items"`
}
```

### InfraMachinePool: instances

Each InfraMachinePool MAY specify a status field that is used to report information about each replica within the machine pool. This field is not used by core CAPI. It is purely informational and is used as convenient way for a user to get details of the replicas in the machine pool, such as their provider id and ip addresses.

If you implement this then create a `status.instances` field that is a slice of a struct type that contains the information you want to store and be made available to the users.

```go
type FooMachinePoolStatus struct {
    // Instances contains the status for each instance in the pool
    // +optional
    Instances []FooMachinePoolInstanceStatus `json:"instances,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}

// FooMachinePoolInstanceStatus contains instance status information about a FooMachinePool.
type FooMachinePoolInstanceStatus struct {
    // Addresses contains the associated addresses for the machine.
    // +optional
    Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

    // InstanceName is the identification of the Machine Instance within the Machine Pool
    InstanceName string `json:"instanceName,omitempty"`

    // ProviderID is the provider identification of the Machine Pool Instance
    // +optional
    ProviderID *string `json:"providerID,omitempty"`

    // Version defines the Kubernetes version for the Machine Instance
    // +optional
    Version *string `json:"version,omitempty"`

    // Ready denotes that the machine is ready
    // +optional
    Ready bool `json:"ready"`
}
```

### MachinePoolMachines support

A provider can opt-in to MachinePool Machines (MPM). With MPM machines all the replicas in a MachinePool are represented by a Machine & InfraMachine. This enables core CAPI to perform common operations on single machines (and their Nodes), such as draining a node before scale down, integration with Cluster Autoscaler and also machine healthchecks.

If you want to adopt MPM then you MUST have a `status.infrastructureMachineKind` field and the field must contain the resource kind of the InfraMachine that represent the replicas in the pool. For example, for the AWS provider the value would be set to `AWSMachine`.

By opting in an infra provider is expected to create a InfraMachine for every replica in the pool. The lifecycle of these InfraMachines must be managed so that when scale up or scale down happens the list of InfraMachines is representative.

```go
type FooMachinePoolStatus struct {
    // InfrastructureMachineKind is the kind of the infrastructure resources behind MachinePool Machines.
    // +optional
    InfrastructureMachineKind string `json:"infrastructureMachineKind,omitempty"`
   
    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

For further information see the [proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220209-machinepool-machines.md).

### InfraMachinePool: providerID

Each InfraMachinePool MAY specify a provider ID on `spec.providerID` that can be used to identify the infrastructure resource that implements the InfraMachinePool.

This field isn't used by core CAPI. Its main purpose is purely informational to the user to surface the infrastructures identifier for the InfraMachinePool. For example, for AWSMachinePool this would be the ASG identifier.

```go
type FooMachinePoolSpec struct {
    // providerID is the identification ID of the FooMachinePool.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=512
    ProviderID string `json:"providerID,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachinePool spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

NOTE: To align with API conventions, we recommend since the v1beta2 contract that the `ProviderID` field should be
of type `string`.

### InfraMachinePool: providerIDList

Each InfraMachinePool MUST supply a list of the identification IDs of the machine instances managed by the machine pool by storing these in `spec.providerIDList`.

```go
type FooMachinePoolSpec struct {
    // ProviderIDList is the list of identification IDs of machine instances managed by this Machine Pool
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MaxItems=10000
    // +kubebuilder:validation:items:MinLength=1
    // +kubebuilder:validation:items:MaxLength=512
    ProviderIDList []string `json:"providerIDList,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachinePool spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Cluster API uses this list to determine the status of the machine pool and to know when replicas have been deleted, at which point the Node will be deleted.

### InfraMachinePool: initialization completed

Each provider MUST indicate when then the InfraMachinePool has been completely provisioned.

Currently this is done by setting `staus.ready` to **true**.  The value retuned here is stored in the MachinePool's `status.infraStructureReady` field.

Additionally providers should set `initialization.provisioned` to **true**. This value isn't currently used by core CAPI for MachinePools. However, MachinePools will start to use this instead and `status.ready` will be deprecated. By setting both these fields it will make the future migration easier.

This indicates to Cluster API that the infrastructure is ready and that it can continue with its processing.

```go
type FooMachinePoolStatus struct {
    // Ready is true when the provider resource is ready.
    // +optional
    Ready bool `json:"ready"`

    // initialization provides observations of the FooMachinePool initialization process.
    // +optional
    Initialization FooMachinePoolInitializationStatus `json:"initialization,omitempty,omitzero"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}

// FooMachinePoolInitializationStatus provides observations of the FooMachinePool initialization process.
// +kubebuilder:validation:MinProperties=1
type FooMachinePoolInitializationStatus struct {
    // provisioned is true when the infrastructure provider reports that the MachinePool's infrastructure is fully provisioned.
    // +optional
    Provisioned *bool `json:"provisioned,omitempty"`
}

```

Once `status.ready` is set the MachinePool “core” controller will bubble up this info in MachinePool’s `status.initialization.infrastructureProvisioned`; also InfraMachinePools’s `spec.providerIDList` and `status.replicas` will be surfaced on MachinePool’s corresponding fields at the same time.

### InfraMachinePool: pausing

Providers SHOULD implement the pause behaviour for every object with a reconciliation loop. This is done by checking if `spec.paused` is set on the Cluster object and by checking for the `cluster.x-k8s.io/paused` annotation on the InfraMachinePool object.

If implementing the pause behaviour, providers SHOULD surface the paused status of an object using the Paused condition: `Status.Conditions[Paused]`.

### InfraMachinePool: conditions

According to [Kubernetes API Conventions], Conditions provide a standard mechanism for higher-level
status reporting from a controller.

Providers implementers SHOULD implement `status.conditions` for their InfraMachinePool resource.
In case conditions are implemented on a InfraMachinePool resource, Cluster API will only consider conditions providing the following information:

- `type` (required)
- `status` (required, one of True, False, Unknown)
- `reason` (optional, if omitted a default one will be used)
- `message` (optional, if omitted an empty message will be used)
- `lastTransitionTime` (optional, if omitted time.Now will be used)
- `observedGeneration` (optional, if omitted the generation of the InfraMachinePool resource will be used)

Other fields will be ignored.

If a condition with type `Ready` exist, such condition will be mirrored in MachinePool’s `InfrastructureReady` condition (not implemented yet).

Please note that the `Ready` condition is expected to surface the status of the InfraMachinePool during its own entire lifecycle, including initial provisioning, the final deletion process, and the period in between these two moments.

See [Improving status in CAPI resources] for more context.

<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

With regards to conditions:

Cluster API will continue to read conditions from providers using deprecated Cluster API condition types.

Please note that provider that will continue to use deprecated Cluster API condition types MUST carefully take into account
the implication of this choice which are described both in the [Cluster API v1.11 migration notes] and in the [Improving status in CAPI resources] proposal.

</aside>

### InfraMachinePool: replicas

Provider implementers MUST implement `status.replicas` to report the most recently observed number of machine instances in the pool. For example, in AWS this would be the number of replicas in a Auto Scale Group (ASG).

```go
type FooMachinePoolStatus struct {
    // Replicas is the most recently observed number of replicas.
    // +optional
    Replicas int32 `json:"replicas"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

The value from this field is surfaced via the MachinePool's `status.replicas` field.

### InfraMachinePool: terminal failures

A provider MAY report failure information via their `status.failureReason` and `status.failureMessage` fields.

```go
type FooMachinePoolStatus struct {
    // FailureReason will be set in the event that there is a terminal problem
    // reconciling the Machine and will contain a succinct value suitable
    // for machine interpretation.
    // +optional
    FailureReason *string `json:"failureReason,omitempty"`

    // FailureMessage will be set in the event that there is a terminal problem
    // reconciling the Machine and will contain a more verbose string suitable
    // for logging and human consumption.
    // +optional
    FailureMessage *string `json:"failureMessage,omitempty"`

    // See other rules for more details about mandatory/optional fields in InfraMachinePool status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

If a provider sets these fields then their value will populated to the same named fields on the the MachinePool.

<aside class="note warning">

<h1>New provider implementations</h1>

The use of `failureReason` and `failureMessage` should not be used for new InfraMachinePool implementations. In other areas of the Cluster API, starting from the v1beta2 contract version, there is no more special treatment for provider's terminal failures within Cluster API. These fields should be regarded as deprecated.

In case necessary, "terminal failures" should be surfaced using conditions, with a well documented type/reason;
it is up to consumers to treat them accordingly.

</aside>

### InfraMachinePoolTemplate, InfraMachineTemplatePoolList resource definition

For a given InfraMachinePool resource, you SHOULD also add a corresponding InfraMachinePoolTemplate resource in order to use it in ClusterClasses. The template resource MUST be name `<InfraMachinePool>Template`.


```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foomachinepooltemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// FooMachinePoolTemplate is the Schema for the foomachinepooltemplates API.
type FooMachinePoolTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec FooMachinePoolTemplateSpec `json:"spec,omitempty"`
}

type FooMachinePoolTemplateSpec struct {
    Template FooMachinePooleTemplateResource `json:"template"`
}

type FooMachinePoolTemplateResource struct {
    // Standard object's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    // +optional
    ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`
    Spec FooMachinePoolSpec `json:"spec"`
}
```

NOTE: in this example `spec.template.spec` embeds `FooMachinePoolSpec` from MachinePool. This might not always be
the best choice depending of if/how InfraMachinePools spec fields applies to many machine pools vs only one.

For each InfraMachinePoolTemplate resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<InfraMachinePoolTemplate>List`.

```go
// +kubebuilder:object:root=true

// FooMachinePoolTemplateList contains a list of FooMachinePoolTemplates.
type FooMachinePoolTemplateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooMachinePoolTemplate `json:"items"`
}
```

### InfraMachinePoolTemplate: support for SSA dry run

When Cluster API's topology controller is trying to identify differences between templates defined in a ClusterClass and
the current Cluster topology, it is required to run [Server Side Apply] (SSA) dry run call.

However, in case you have immutability checks for your InfraMachinePoolTemplate, this can lead the SSA dry run call to error.

In order to avoid this InfraMachinePoolTemplate MUST specifically implement support for SSA dry run calls from the topology controller.

The implementation requires to use controller runtime's `CustomValidator`, available in CR versions >= v0.12.3.

This will allow to skip the immutability check only when the topology controller is dry running while preserving the
validation behavior for all other cases.

### Multi tenancy

Multi tenancy in Cluster API defines the capability of an infrastructure provider to manage different credentials,
each one of them corresponding to an infrastructure tenant.

See [infrastructure Provider Security Guidance] for considerations about cloud provider credential management.

Please also note that Cluster API does not support running multiples instances of the same provider, which someone can
assume an alternative solution to implement multi tenancy; same applies to the clusterctl CLI.

See [Support running multiple instances of the same provider] for more context.

However, if you want to make it possible for users to run multiples instances of your provider, your controller's SHOULD:

- support the `--namespace` flag.
- support the `--watch-filter` flag.

Please, read carefully the page linked above to fully understand implications and risks related to this option.

### Clusterctl support

The clusterctl command is designed to work with all the providers compliant with the rules defined in the [clusterctl provider contract].

[All resources: Scope]: #all-resources-scope
[All resources: `TypeMeta` and `ObjectMeta`field]: #all-resources-typemeta-and-objectmeta-field
[All resources: `APIVersion` field value]: #all-resources-apiversion-field-value
[InfraMachinePool, InfraMachinePoolList resource definition]: #inframachinepool-inframachinepoollist-resource-definition
[InfraMachinePool: instances]: #inframachinepool-instances
[InfraMachinePool: providerID]: #inframachinepool-providerid
[InfraMachinePool: providerIDList]: #inframachinepool-provideridlist
[InfraMachinePool: initialization completed]: #inframachinepool-initialization-completed
[InfraMachinePool: pausing]: #inframachinepool-pausing
[InfraMachinePool: conditions]: #inframachinepool-conditions
[InfraMachinePool: replicas]: #inframachinepool-replicas
[InfraMachinePool: terminal failures]: #inframachinepool-terminal-failures
[InfraMachinePoolTemplate, InfraMachineTemplatePoolList resource definition]: #inframachinepooltemplate-inframachinetemplatepoollist-resource-definition
[InfraMachinePoolTemplate: support for SSA dry run]: #inframachinepooltemplate-support-for-ssa-dry-run
[MachinePoolMachines support]: #machinepoolmachines-support
[Multi tenancy]: #multi-tenancy
[Clusterctl support]: #clusterctl-support
[Kubernetes API Conventions]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
[Improving status in CAPI resources]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md
[infrastructure Provider Security Guidance]: ../security-guidelines.md
[Support running multiple instances of the same provider]: ../../core/support-multiple-instances.md
[clusterctl provider contract]: clusterctl.md