# Contract rules for ControlPlane

Control plane providers MUST implement a ControlPlane resource using Kubernetes' CustomResourceDefinition (CRD).

The goal of a ControlPlane resource is to instantiate a Kubernetes control plane; a Kubernetes control plane
at least contains the following components:
- Kubernetes API Server
- Kubernetes Controller Manager
- Kubernetes Scheduler
- etcd (if not externally managed)

Optional control plane components are
- Cloud controller manager
- Cluster DNS (e.g. CoreDNS)
- Service proxy (e.g. kube-proxy)

Instead, CNI should be left to users to apply once the control plane is instantiated.

The ControlPlane resource will be referenced by one of the Cluster API core resources, Cluster.

The [Cluster's controller](../../core/controllers/cluster.md) will be responsible to coordinate operations of the ControlPlane,
and the interaction between the Cluster's controller and the ControlPlane resource is based on the contract
rules defined in this page.

Once contract rules are satisfied by a ControlPlane implementation, other implementation details
could be addressed according to the specific needs (Cluster API is not prescriptive).

Nevertheless, it is always recommended to take a look at Cluster API controllers,
in-tree providers, other providers and use them as a reference implementation (unless custom solutions are required
in order to address very specific needs).

In order to facilitate the initial design for each ControlPlane resource, a few [implementation best practices]
are explicitly called out in dedicated pages. 

On top of that special consideration MUST be done to ensure
security around private key material required to create and run the Kubernetes control plane.

<aside class="note warning">

<h1>Never rely on Cluster API behaviours not defined as a contract rule!</h1>

When developing a provider, you MUST consider any Cluster API behaviour that is not defined by a contract rule
as a Cluster API internal implementation detail, and internal implementation details can change at any time.

Accordingly, in order to not expose users to the risk that your provider breaks when the Cluster API internal behavior
changes, you MUST NOT rely on any Cluster API internal behaviour when implementing a ControlPlane resource.

Instead, whenever you need something more from the Cluster API contract, you MUST engage the community.

The Cluster API maintainers welcome feedback and contributions to the contract in order to improve how it's defined,
its clarity and visibility to provider implementers and its suitability across the different kinds of Cluster API providers.

To provide feedback or open a discussion about the provider contract please [open an issue on the Cluster API](https://github.com/kubernetes-sigs/cluster-api/issues/new?assignees=&labels=&template=feature_request.md)
repo or add an item to the agenda in the [Cluster API community meeting](https://git.k8s.io/community/sig-cluster-lifecycle/README.md#cluster-api).

</aside>

## Rules (contract version v1beta2)

| Rule                                                                 | Mandatory | Note                                                                                                                       |
|----------------------------------------------------------------------|-----------|----------------------------------------------------------------------------------------------------------------------------|
| [All resources: scope]                                               | Yes       |                                                                                                                            |
| [All resources: `TypeMeta` and `ObjectMeta`field]                    | Yes       |                                                                                                                            |
| [All resources: `APIVersion` field value]                            | Yes       |                                                                                                                            |
| [ControlPlane, ControlPlaneList resource definition]                 | Yes       |                                                                                                                            |
| [ControlPlane: endpoint]                                             | No        | Mandatory if control plane endpoint is not provided by other means.                                                        |
| [ControlPlane: replicas]                                             | No        | Mandatory if control plane has a notion of number of instances.                                                            |
| [ControlPlane: version]                                              | No        | Mandatory if control plane allows direct management of the Kubernetes version in use; Mandatory for cluster class support. |
| [ControlPlane: machines]                                             | No        | Mandatory if control plane instances are represented with a set of Cluster API Machines.                                   |
| [ControlPlane: initialization completed]                             | Yes       |                                                                                                                            |
| [ControlPlane: in-place updates]                                     | No        | Only supported for control plane providers with control plane machines                                                     |
| [ControlPlane: conditions]                                           | No        |                                                                                                                            |
| [ControlPlane: terminal failures]                                    | No        |                                                                                                                            |
| [ControlPlaneTemplate, ControlPlaneTemplateList resource definition] | No        | Mandatory for ClusterClasses support                                                                                       |
| [Cluster kubeconfig management]                                      | Yes       |                                                                                                                            |
| [Cluster certificate management]                                     | No        |                                                                                                                            |
| [Machine placement]                                                  | No        |                                                                                                                            |
| [Metadata propagation]                                               | No        |                                                                                                                            |
| [MinReadySeconds and UpToDate propagation]                           | No        |                                                                                                                            |
| [Support for running multiple instances]                             | No        | Mandatory for clusterctl CLI support                                                                                       |
| [Clusterctl support]                                                 | No        | Mandatory for clusterctl CLI support                                                                                       |
| [ControlPlane: pausing]                                              | No        |                                                                                                                            |

### All resources: scope

All resources MUST be namespace-scoped.

### All resources: `TypeMeta` and `ObjectMeta` field

All resources MUST have the standard Kubernetes `TypeMeta` and `ObjectMeta` fields.

### All resources: `APIVersion` field value

In Kubernetes `APIVersion` is a combination of API group and version.
Special consideration MUST applies to both API group and version for all the resources Cluster API interacts with.

#### All resources: API group

The domain for Cluster API resources is `cluster.x-k8s.io`, and control plane providers under the Kubernetes SIGS org
generally use `controlplane.cluster.x-k8s.io` as API group.

If your provider uses a different API group, you MUST grant full read/write RBAC permissions for resources in your API group
to the Cluster API core controllers. The canonical way to do so is via a `ClusterRole` resource with the [aggregation label]
`cluster.x-k8s.io/aggregate-to-manager: "true"`.

The following is an example ClusterRole for a `FooControlPlane` resource in the `controlplane.foo.com` API group:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: capi-foo-controlplane
    labels:
      cluster.x-k8s.io/aggregate-to-manager: "true"
rules:
- apiGroups:
    - controlplane.foo.com
  resources:
    - foocontrolplanes
  verbs:
    - create
    - delete
    - get
    - list
    - patch
    - update
    - watch
- apiGroups:
    - controlplane.foo.com
  resources:
    - foocontrolplanetemplates
  verbs:
    - get
    - list
    - patch
    - update
    - watch
```

Note: The write permissions allow the Cluster controller to set owner references and labels on the ControlPlane resources;
write permissions are not used for general mutations of ControlPlane resources, unless specifically required (e.g. when
using ClusterClass and managed topologies).

#### All resources: version

The resource Version defines the stability of the API and its backward compatibility guarantees.
Examples include `v1alpha1`, `v1beta1`, `v1`, etc. and are governed by the [Kubernetes API Deprecation Policy].

Your provider SHOULD abide by the same policies.

Note: The version of your provider does not need to be in sync with the version of core Cluster API resources.
Instead, prefer choosing a version that matches the stability of the provider API and its backward compatibility guarantees.

Additionally:

Providers MUST set `cluster.x-k8s.io/<version>` label on the InfraCluster Custom Resource Definitions.

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

An example of this is in the [Kubeadm Bootstrap provider](https://github.com/kubernetes-sigs/cluster-api/blob/release-1.1/controlplane/kubeadm/config/crd/kustomization.yaml).

<aside  class="note warning">

<h1>Important</h1>

If the provider implements the [clusterctl provider contract], the contract version defined in the
label above must be consistent with the contract version defined in the `metadata.yaml` file.

</aside>

### ControlPlane, ControlPlaneList resource definition

You MUST define a ControlPlane resource.
The ControlPlane resource name must have the format produced by `sigs.k8s.io/cluster-api/util/contract.CalculateCRDName(Group, Kind)`.

Note: Cluster API is using such a naming convention to avoid an expensive CRD lookup operation when looking for labels from
the CRD definition of the ControlPlane resource.

It is a generally applied convention to use names in the format `${env}ControlPlane`, where ${env} is a, possibly short, name
for the control plane implementation in question. For example `KubeadmControlPlane` is an implementation of a control plane
using kubeadm as a bootstrapper tool.

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foocontrolplanes,shortName=foocp,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// FooControlPlane is the Schema for foocontrolplanes.
type FooControlPlane struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec FooControlPlaneSpec `json:"spec,omitempty"`
    Status FooControlPlaneStatus `json:"status,omitempty"`
}

type FooControlPlaneSpec struct {
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

type FooControlPlaneStatus struct {
    // See other rules for more details about mandatory/optional fields in ControlPlane status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

For each ControlPlane resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<ControlPlane>List`.

```go
// +kubebuilder:object:root=true

// FooControlPlaneList contains a list of foocontrolplanes.
type FooControlPlaneList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooControlPlane `json:"items"`
}
```

### ControlPlane: endpoint

Each Cluster needs a control plane endpoint to sit in front of control plane machines.
Control plane endpoint can be provided in three ways in Cluster API: by the users, by the control plane provider or
by the infrastructure provider.

In case you are developing a control plane provider which is responsible to provide a control plane endpoint for
each Cluster, the host and port of the generated control plane endpoint MUST surface on `spec.controlPlaneEndpoint`
in the ControlPlane resource.

```go
type FooControlPlaneSpec struct {
    // controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty,omitzero"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
// +kubebuilder:validation:MinProperties=1
type APIEndpoint struct {
    // host is the hostname on which the API server is serving.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=512
    Host string `json:"host,omitempty"`

    // port is the port on which the API server is serving.
    // +optional
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=65535
    Port int32 `json:"port,omitempty"`
}
```

Once `spec.controlPlaneEndpoint` is set on the ControlPlane resource and the [ControlPlane: initialization completed],
the Cluster controller will surface this info in Cluster's `spec.controlPlaneEndpoint`.

If instead you are developing a control plane provider which is NOT responsible to provide a control plane endpoint,
the implementer should exit reconciliation until it sees Cluster's `spec.controlPlaneEndpoint` populated.

<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

```go
type FooControlPlaneSpec struct {
    // controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
    // host is the hostname on which the API server is serving.
    Host string `json:"host"`
    
    // port is the port on which the API server is serving.
    Port int32 `json:"port"`
}
```

</aside>

### ControlPlane: replicas

In case you are developing a control plane provider which allows control of the number of replicas of the
Kubernetes control plane instances in your control plane, following fields MUST be implemented in
the ControlPlane `spec`.

```go
type FooControlPlaneSpec struct {
    // replicas represent the number of desired replicas.
    // This is a pointer to distinguish between explicit zero and not specified.
    // +optional
    Replicas *int32 `json:"replicas,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Following fields MUST be implemented in the ControlPlane `status`.

```go
type FooControlPlaneStatus struct {
    // selector is the label selector in string format to avoid introspection
    // by clients, and is used to provide the CRD-based integration for the
    // scale subresource and additional integrations for things like kubectl
    // describe. The string will be in the same format as the query-param syntax.
    // More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
    // +optional
    Selector string `json:"selector,omitempty"`

    // replicas is the total number of machines targeted by this control plane
    // (their labels match the selector).
    // +optional
    Replicas *int32 `json:"replicas,omitempty"`

    // readyReplicas is the number of ready replicas for this ControlPlane. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

    // availableReplicas is the number of available replicas for this ControlPlane. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

    // upToDateReplicas is the number of up-to-date replicas targeted by this ControlPlane. A machine is considered available when Machine's  UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

    // See other rules for more details about mandatory/optional fields in ControlPlane status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

As you might have already noticed from the `status.selector` field, the ControlPlane custom resource definition MUST
support the `scale` subresource with the following signature:

``` yaml
scale:
  labelSelectorPath: .status.selector
  specReplicasPath: .spec.replicas
  statusReplicasPath: .status.replicas
status: {}
```

More information about the [scale subresource can be found in the Kubernetes
documentation][scale].


<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

With regards to v1beta1 replica counters, the Cluster controller with temporarily continue to read
`status.readyReplicas`,  `status.updatedReplicas` and `status.unavailableReplicas`, even if the semantic of the 
field might be different from what expected.

</aside>

### ControlPlane: version

In case you are developing a control plane provider which allows control of the version of the
Kubernetes control plane instances in your control plane, following fields MUST be implemented in
the ControlPlane `spec`.

```go
type FooControlPlaneSpec struct {
    // version defines the desired Kubernetes version for the control plane. 
    // The value must be a valid semantic version; also if the value provided by the user does not start with the v prefix, it
    // must be added.
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=256
    Version string `json:"version"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Following fields MUST be implemented in the ControlPlane `status`.

```go
type FooControlPlaneStatus struct {
    // version represents the minimum Kubernetes version for the control plane machines
    // in the cluster.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=256
    Version string `json:"version,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

NOTE: To align with API conventions, we recommend since the v1beta2 contract that the `Version` field should be 
of type `string` (it was `*string` before). Both are compatible with the v1beta2 contract though.
NOTE: The minimum Kubernetes version, and more specifically the API server version, will be used to determine 
when a control plane is fully upgraded (spec.version == status.version) and for enforcing Kubernetes version skew 
policies when a Cluster derived from a ClusterClass is managed by the Topology controller.

### ControlPlane: machines

In case you are developing a control plane provider which uses a Cluster API Machine object to represent each
control plane instance, the providers MUST set the `cluster.x-k8s.io/control-plane` label with an empty value on the created Machines.

Additionally following the fields MUST be implemented in
the ControlPlane `spec`.

```go
type FooControlPlaneSpec struct {
    // machineTemplate contains information about how machines
    // should be shaped when creating or updating a control plane.
    // +required
    MachineTemplate FooControlPlaneMachineTemplate `json:"machineTemplate"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

type FooControlPlaneMachineTemplate struct {
    // metadata is the standard object's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    // +optional
    ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

    // spec defines the spec for Machines of the control plane.
    // +optional
    Spec FooControlPlaneMachineTemplateSpec `json:"spec,omitempty,omitzero"`
}

type FooControlPlaneMachineTemplateSpec struct {
	// infrastructureRef is a required reference to a custom infra machine template resource
	// offered by an infrastructure provider.
	// +required
	InfrastructureRef clusterv1.ContractVersionedObjectReference `json:"infrastructureRef"`

	// deletion contains configuration options for Machine deletion.
	// +optional
	Deletion FooControlPlaneMachineTemplateDeletionSpec `json:"deletion,omitempty,omitzero"`
}

// FooControlPlaneMachineTemplateDeletionSpec contains configuration options for Machine deletion.
// +kubebuilder:validation:MinProperties=1
type FooControlPlaneMachineTemplateDeletionSpec struct {
    // nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a controlplane node
    // The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`
    // +optional
    // +kubebuilder:validation:Minimum=0
    NodeDrainTimeoutSeconds *int32 `json:"nodeDrainTimeoutSeconds,omitempty"`
    
    // nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes
    // to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
    // +optional
    // +kubebuilder:validation:Minimum=0
    NodeVolumeDetachTimeoutSeconds *int32 `json:"nodeVolumeDetachTimeoutSeconds,omitempty"`
    
    // nodeDeletionTimeoutSeconds defines how long the machine controller will attempt to delete the Node that the Machine
    // hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
    // If no value is provided, the default value for this property of the Machine resource will be used.
    // +optional
    // +kubebuilder:validation:Minimum=0
    NodeDeletionTimeoutSeconds *int32 `json:"nodeDeletionTimeoutSeconds,omitempty"`
  
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Please note that some of the above fields (`metadata`, `nodeDrainTimeoutSeconds`, `nodeVolumeDetachTimeoutSeconds`, `nodeDeletionTimeoutSeconds`)
must be propagated to machines without triggering rollouts.
See [In place propagation of changes affecting Kubernetes objects only] as well as [Metadata propagation] for more details.

<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

For reference. The v1beta1 contract had `nodeDrainTimeout`, `nodeVolumeDetachTimeout`, `nodeDeletionTimeout` fields
of type `*metav1.Duration` instead of the new fields with `Seconds` suffix of type `*int32`.
`infrastructureRef` was of type `corev1.ObjectReference`, the new field has the type  `clusterv1.ContractVersionedObjectReference`.
In v1beta2 we also moved `infrastructureRef` into the newly introduced `spec.machineTemplate.spec` field and `nodeDrainTimeoutSeconds`,
`nodeVolumeDetachTimeoutSeconds` and `nodeDeletionTimeoutSeconds` into the newly introduced `spec.machineTemplate.spec.deletion` field.

</aside>

In case you are developing a control plane provider that allows definition of machine readiness gates, you SHOULD also implement
the following `spec.machineTemplate.spec` field.

```go
type FooControlPlaneMachineTemplateSpec struct {
    // readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
    //
    // This field can be used e.g. by Cluster API control plane providers to extend the semantic of the
    // Ready condition for the Machine they control, like the kubeadm control provider adding ReadinessGates
    // for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
    //
    // Another example are external controllers, e.g. responsible to install special software/hardware on the Machines;
    // they can include the status of those components with a new condition and add this condition to ReadinessGates.
    //
    // NOTE: This field is considered only for computing v1beta2 conditions.
    // NOTE: In case readinessGates conditions start with the APIServer, ControllerManager, Scheduler prefix, and all those
    // readiness gates condition are reporting the same message, when computing the Machine's Ready condition those
    // readinessGates will be replaced by a single entry reporting "Control plane components: " + message.
    // This helps to improve readability of conditions bubbling up to the Machine's owner resource / to the Cluster).
    // +optional
    // +listType=map
    // +listMapKey=conditionType
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=32
    ReadinessGates []clusterv1.MachineReadinessGate `json:"readinessGates,omitempty"`

    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

NOTE: In the v1beta1 contract the `readinessGates` field was located directly in the `spec.machineTemplate` field.

In case you are developing a control plane provider that allows definition of machine taints, you SHOULD also implement
the following `spec.machineTemplate.spec` field.

```go
type FooControlPlaneMachineTemplateSpec struct {
	// taints are the node taints that Cluster API will manage.
	// This list is not necessarily complete: other Kubernetes components may add or remove other taints from nodes,
	// e.g. the node controller might add the node.kubernetes.io/not-ready taint.
	// Only those taints defined in this list will be added or removed by core Cluster API controllers.
	//
	// There can be at most 64 taints.
	// A pod would have to tolerate all existing taints to run on the corresponding node.
	//
	// NOTE: This list is implemented as a "map" type, meaning that individual elements can be managed by different owners.
	// +optional
	// +listType=map
	// +listMapKey=key
	// +listMapKey=effect
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	Taints []clusterv1.MachineTaint `json:"taints,omitempty"`

    // See other rules for more details about mandatory/optional fields in ControlPlane spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

In case you are developing a control plane provider where control plane instances uses a Cluster API Machine 
object to represent each control plane instance, but those instances do not show up as a Kubernetes node (for example, 
managed control plane providers for AKS, EKS, GKE etc), you SHOULD also implement the following `status` field.

```go
type FooControlPlaneStatus struct {
    // externalManagedControlPlane is a bool that should be set to true if the Node objects do not exist in the cluster.
    // +optional
    ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

    // See other rules for more details about mandatory/optional fields in ControlPlane status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

NOTE: To align with API conventions, we recommend since the v1beta2 contract that the `ExternalManagedControlPlane` field should be
of type `*bool` (it was `bool` before). Both are compatible with the v1beta2 contract though.

Please note that by representing each control plane instance as Cluster API machine, each control plane instance
can benefit from several Cluster API behaviours, for example:
- Machine provisioning workflow (in coordination with an InfraMachine and a BootstrapConfig of your choice)
- Machine health checking
- Machine drain and wait for volume detach during deletion

### ControlPlane: initialization completed

Each ControlPlane MUST report when the Kubernetes control plane is initialized; usually a control plane is considered
initialized when it can accept requests, no matter if this happens before the control plane is fully provisioned or not.

For example, in a highly available Kubernetes control plane with three instances of each component, usually the control plane
can be considered initialized after the first instance is up and running.

A ControlPlane reports when it is initialized by setting `status.initialization.controlPlaneInitialized`.

```go
type FooControlPlaneStatus struct {
    // initialization provides observations of the FooControlPlane initialization process.
    // NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
    // +optional
    Initialization FooControlPlaneInitializationStatus `json:"initialization,omitempty,omitzero"`
    
    // See other rules for more details about mandatory/optional fields in ControlPlane status.
    // Other fields SHOULD be added based on the needs of your provider.
}

// FooControlPlaneInitializationStatus provides observations of the FooControlPlane initialization process.
// +kubebuilder:validation:MinProperties=1
type FooControlPlaneInitializationStatus struct {
    // controlPlaneInitialized is true when the control plane provider reports that the Kubernetes control plane is initialized; 
    // usually a control plane is considered initialized when it can accept requests, no matter if this happens before 
    // the control plane is fully provisioned or not.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Cluster provisioning.
    // +optional
    ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}
```

Once `status.initialization.controlPlaneInitialized` is set the Cluster "core" controller will bubble up this info in Cluster's
`status.initialization.controlPlaneInitialized` field and in the `ControlPlaneInitialized` condition.

If defined, also ControlPlane's `spec.controlPlaneEndpoint` will be surfaced on Cluster's corresponding fields at the same time.

<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

With regards to initialization completed:

Cluster API will continue to temporarily support ControlPlane resource using the `status.initialized` field to
report initialization completed.

After compatibility with the deprecated v1beta1 contract will be removed, the `status.initialized` field in
the ControlPlane resource will be ignored.

</aside>

### ControlPlane: in-place updates

In case a control plane provider would like to provide support for in-place updates, please check the [proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/20240807-in-place-updates.md).

Supporting in-place updates requires:
- implementing the call for the registered `CanUpdateMachine` hook when performing the "can update in-place" decision.
- when it is decided to perform the in-place decision:
  - the machine spec must be updated to the desired state, as well as the spec for the corresponding infrastructure machine and bootstrap config
  - while updating those objects also the `in-place-updates.internal.cluster.x-k8s.io/update-in-progress` annotation must be set
  - once all objects are updated the `UpdateMachine` hook must be set as pending on the machine object

After above steps are completed, the machine controller will take over and complete the in-place upgrade.

<aside class="note warning">

<h1>High complexity</h1>

Implementing the in-place update transition in a race condition-free, re-entrant way is more complex than it might seem.

Please read the proposal's [implementation notes](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240807-in-place-updates-implementation-notes.md)
carefully.

Also, it is highly recommended to use the KCP implementation as a reference.

</aside>


### ControlPlane: conditions

According to [Kubernetes API Conventions], Conditions provide a standard mechanism for higher-level
status reporting from a controller.

Providers implementers SHOULD implement `status.conditions` for their ControlPlane resource.
In case conditions are implemented on a ControlPlane resource, Cluster API will only consider conditions providing the following information:
- `type` (required)
- `status` (required, one of True, False, Unknown)
- `reason` (optional, if omitted a default one will be used)
- `message` (optional, if omitted an empty message will be used)
- `lastTransitionTime` (optional, if omitted time.Now will be used)
- `observedGeneration` (optional, if omitted the generation of the ControlPlane resource will be used)

Other fields will be ignored.

If a condition with type `Available` exist, such condition will be mirrored in Cluster's `ControlPlaneAvailable` condition.

The `Available` condition is expected to properly represents the fact that a ControlPlane can be operational
even if there is a certain degree of not readiness / disruption in the system, or if lifecycle operations are happening.

Last, but not least, in order to ensure a consistent users experience, it is also recommended to consider aligning also other
ControlPlane conditions to conditions existing on other Cluster API objects.

For example `KubeadmControlPlane` implements the following conditions on top of the `Available` defined by this contract:
`CertificatesAvailable`, `EtcdClusterAvailable`, `MachinesReady`, `MachinesUpToDate`, `RollingOut`, `ScalingUp`, `ScalingDown`,
`Remediating`, `Deleting`, `Paused`.

Most notably, If `RollingOut`, `ScalingUp`, `ScalingDown` conditions are implemented, the Cluster controller is going to read
them to compute a Cluster level `RollingOut`, `ScalingUp`, `ScalingDown` condition including all the scalable resources.

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

### ControlPlane: terminal failures

Starting from the v1beta2 contract version, there is no more special treatment for provider's terminal failures within Cluster API.

In case necessary, "terminal failures" should be surfaced using conditions, with a well documented type/reason;
it is up to consumers to treat them accordingly.

See [Improving status in CAPI resources] for more context.

<aside class="note warning">

<h1>Compatibility with the deprecated v1beta1 contract</h1>

In order to ease the transition for providers, the v1beta2 version of the Cluster API contract _temporarily_
preserves compatibility with the deprecated v1beta1 contract; compatibility will be removed tentatively in August 2026.

With regards to terminal failures:

In case a Control Plane provider reports that a ControlPlane resource is in a state that cannot be recovered (terminal failure) by
setting `status.failureReason` and `status.failureMessage` as defined by the deprecated v1beta1 contract,
the "core" Cluster controller will surface those info in the corresponding fields in the Cluster's `status.deprecated.v1beta1` struct.

However, those info won't have any impact on the Cluster lifecycle as before (the Cluster controller won't consider the
presence of `status.failureReason` and `status.failureMessage` info as "terminal failures").

After compatibility with the deprecated v1beta1 contract will be removed, `status.failureReason` and `status.failureMessage`
fields in the ControlPlane resource will be ignored and Cluster's `status.deprecated.v1beta1` struct will be dropped.

</aside>

### ControlPlaneTemplate, ControlPlaneTemplateList resource definition

For a given ControlPlane resource, you should also add a corresponding ControlPlaneTemplate resources in order to use it in ClusterClasses.
The template resource MUST be named as `<ControlPlane>Template`.

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foocontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// FooControlPlaneTemplate is the Schema for the fooclustertemplates API.
type FooControlPlaneTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec FooControlPlaneTemplateSpec `json:"spec,omitempty"`
}

type FooControlPlaneTemplateSpec struct {
    Template FooControlPlaneTemplateResource `json:"template"`
}

type FooControlPlaneTemplateResource struct {
    // Standard object's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    // +optional
    ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`
    Spec FooControlPlaneSpec `json:"spec"`
}
```

NOTE: in this example ControlPlaneTemplate's `spec.template.spec` embeds `FooControlPlaneSpec` from ControlPlane. This might not always be
the best choice depending of if/how ControlPlane's spec fields applies to many clusters vs only one.

For each ControlPlaneTemplate resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<ControlPlaneTemplate>List`.

```go
// +kubebuilder:object:root=true

// FooControlPlaneTemplateList contains a list of FooControlPlaneTemplates.
type FooControlPlaneTemplateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooControlPlaneTemplate `json:"items"`
}
```

### Cluster kubeconfig management

Control Plane providers are expected to create and maintain a Kubeconfig secret for Cluster API to gain access to the
workload cluster.

Such secret might be used also by operators to gain initial access to the cluster, but this secret MUST not be shared
with other users or applications build on top of Cluster API. Instead, follow instruction in [Certificate Management](https://cluster-api.sigs.k8s.io/tasks/certs/)
to create custom certificates for additional users or other applications.

The kubeconfig secret MUST:
- Be created in the same namespace where the Cluster exists
- Be named `<cluster>-kubeconfig`
- Have type `cluster.x-k8s.io/secret`
- Be labelled with the key-pair `cluster.x-k8s.io/cluster-name=${CLUSTER_NAME}`.
  Note: this label is required for the secret to be retrievable in the cache used by CAPI managers.
- Have the base64 encoded kubeconfig in the field called `value`

Important! If a control plane provider uses client certificates for authentication in these Kubeconfigs, the client certificate 
MUST be kept with a reasonably short expiration period and periodically regenerated to keep a valid set of credentials available.
As an example, the Kubeadm Control Plane provider uses a year of validity and refreshes the certificate after 6 months.

### Cluster certificate management

Control Plane providers are expected to create and maintain all the certificates required to create and run a Kubernetes cluster.

Cluster certificates MUST be stored as a secrets:
- In the same namespace where the Cluster exists
- Following a naming convention `<cluster>-<certificate>`; common certificate names are `ca`, `etcd`, `proxy`, `sa`  
- Have type `cluster.x-k8s.io/secret`
- Be labelled with the key-pair `cluster.x-k8s.io/cluster-name=${CLUSTER_NAME}`.
  Note: this label is required for the secret to be retrievable in the cache used by CAPI managers.

See [Certificate Management] for more context.

### Machine placement

Control Plane providers are expected to place machines in failure domains defined in Cluster's `status.failureDomains` field.

More specifically, Control Plane should be spread across failure domains specifically flagged to host control plane machines.

### Metadata propagation

Cluster API defines rules to propagate metadata (labels and annotations) across the hierarchies of objects, down
to Machines and nodes.

In order to ensure a nice and consistent user experience across the entire Cluster, also ControlPlane providers
are expected to implement similar propagation rules for control plane machines.

See. [Metadata propagation rules] for more 
details about how metadata should be propagated across the hierarchy of Cluster API objects (use KubeadmControlPlane as a reference).

Also, please note that `metadata` MUST be propagated to control plane instances machines without triggering rollouts.
See [In place propagation of changes affecting Kubernetes objects only] for more details.

See. [Label and Annotations Sync Between Machines and underlying Kubernetes Nodes] for more details about how metadata are
propagated to Kubernetes Nodes.

### MinReadySeconds and UpToDate propagation

<aside class="note warning">

<h1>Heads up! this will change with the v1beta2 contract</h1>

When the v1beta2 contract will be released (tentative Apr 2025), Cluster API is going to standardize how
machines determine if they are available or up to date with the spec of the owner resource.

In order to ensure a nice and consistent user experience across the entire Cluster, also ControlPlane providers
are expected to align to this effort and implement the following behaviours:

Control plane providers will be expected to continuously set Machines `spec.minReadySeconds` and Machine's 
`status.conditions[UpToDate]` condition.

Please note that a CP provider implementation can decide to enforce `spec.minReadySeconds` to be 0 and do not
introduce a difference between readiness and availability or introduce it at a later stage (e.g. KCP will do this). 

Additionally, please note that the `spec.minReadySeconds` field MUST be treated like other fields propagated /updated in place, 
and thus propagated to Machines without triggering rollouts.

See [Improving status in CAPI resources] and [In place propagation of changes affecting Kubernetes objects only] for more context.

</aside>

### Support for running multiple instances

Cluster API does not support running multiples instances of the same provider, which someone can
assume an alternative solution to implement multi tenancy; same applies to the clusterctl CLI.

See [Support running multiple instances of the same provider] for more context.

However, if you want to make it possible for users to run multiples instances of your provider, your controller's SHOULD:

- support the `--namespace` flag.
- support the `--watch-filter` flag.

Please, read carefully the page linked above to fully understand implications and risks related to this option.

### Clusterctl support

The clusterctl command is designed to work with all the providers compliant with the rules defined in the [clusterctl provider contract].

### ControlPlane: pausing

Providers SHOULD implement the pause behaviour for every object with a reconciliation loop. This is done by checking if `spec.paused` is set on the Cluster object and by checking for the `cluster.x-k8s.io/paused` annotation on the ControlPlane object.

If implementing the pause behavior, providers SHOULD surface the paused status of an object using the Paused condition: `Status.Conditions[Paused]`.

## Typical ControlPlane reconciliation workflow

A control plane provider must respond to changes to its ControlPlane resources. This process is
typically called reconciliation. The provider must watch for new, updated, and deleted resources and respond
accordingly.

As a reference you can look at the following workflow to understand how the typical reconciliation workflow
is implemented in ControlPlane controllers:

![](../../../images/control-plane-controller.png)

[All resources: Scope]: #all-resources-scope
[All resources: `TypeMeta` and `ObjectMeta`field]: #all-resources-typemeta-and-objectmeta-field
[All resources: `APIVersion` field value]: #all-resources-apiversion-field-value
[aggregation label]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#aggregated-clusterroles
[Kubernetes API Deprecation Policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/
[ControlPlane, ControlPlaneList resource definition]: #controlplane-controlplanelist-resource-definition
[ControlPlane: endpoint]: #controlplane-endpoint 
[ControlPlane: replicas]: #controlplane-replicas 
[scale]: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#subresources
[ControlPlane: machines]: #controlplane-machines
[In place propagation of changes affecting Kubernetes objects only]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20221003-In-place-propagation-of-Kubernetes-objects-only-changes.md
[ControlPlane: version]: #controlplane-version 
[ControlPlane: initialization completed]: #controlplane-initialization-completed
[ControlPlane: in-place updates]: #controlplane-in-place-updates
[ControlPlane: conditions]: #controlplane-conditions 
[Kubernetes API Conventions]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
[Improving status in CAPI resources]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md
[ControlPlane: terminal failures]: #controlplane-terminal-failures 
[ControlPlaneTemplate, ControlPlaneTemplateList resource definition]: #controlplanetemplate-controlplanetemplatelist-resource-definition
[Cluster kubeconfig management]: #cluster-kubeconfig-management
[Certificate Management]: https://cluster-api.sigs.k8s.io/tasks/certs/
[Cluster certificate management]: #cluster-certificate-management
[Machine placement]: #machine-placement
[Metadata propagation]: #metadata-propagation
[Metadata propagation rules]: https://main.cluster-api.sigs.k8s.io/reference/api/metadata-propagation
[Label and Annotations Sync Between Machines and underlying Kubernetes Nodes]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220927-labels-and-annotations-sync-between-machine-and-nodes.md
[MinReadySeconds and UpToDate propagation]: #minreadyseconds-and-uptodate-propagation
[Support for running multiple instances]: #support-for-running-multiple-instances
[Support running multiple instances of the same provider]: ../../core/support-multiple-instances.md
[Clusterctl support]: #clusterctl-support
[ControlPlane: pausing]: #controlplane-pausing
[implementation best practices]: ../best-practices.md
[Cluster API v1.11 migration notes]: ../migrations/v1.10-to-v1.11.md
