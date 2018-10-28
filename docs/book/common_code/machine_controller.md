
# Machine Controller

A `Machine` is the declarative spec for a Node, as represented in Kubernetes 
core. If a new Machine object is created, a provider-specific controller will 
handle provisioning and installing a new host to register as a new Node matching
the Machine spec. If the Machine's spec is updated, a provider-specific 
controller is responsible for updating the Node in-place or replacing the host 
with a new one matching the updated spec. If a Machine object is deleted, the 
corresponding Node should have its external resources released by the provider-
specific controller, and should be deleted as well.

Fields like the kubelet version, the container runtime to use, and its version,
are modeled as fields on the Machine's spec. Any other information that is 
provider-specific, though, is part of an opaque `ProviderSpec` string that is 
not portable between different providers.

{% panel style="info", title="Important" %}
To see the actual and current definitions see the [source](#machine_types_source).
{% endpanel %}

{% method %}
## Machine

`Machine` has 4 fields:

Spec contains the desired cluster state specified by the object. While much
of the Spec is defined by users, unspecified parts may be filled in with
defaults or by Controllers such as autoscalers.

Status contains only observed cluster state and is only written by controllers
Status is not the source of truth for any information, but instead aggregates
and publishes observed state.

TypeMeta contains metadata about the API itself - such as Group, Version, Kind.
ObjectMeta contains metadata about the specific object instance - such as the
name, namespace, labels and annotations. ObjectMeta contains data common to most
objects.

{% sample lang="go" %}
```go
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}
```
{% endmethod %}

{% method %}
## MachineSpec

The `ProviderSpec` is recommended to be a serialized API object in a format 
owned by that provider. This will allow the configuration to be strongly typed,
versioned, and have as much nested depth as appropriate. These provider-specific
API definitions are meant to live outside of the Machines API, which will allow
them to evolve independently of it. Attributes like instance type, which 
network to use, and the OS image all belong in the `ProviderSpec`.

Some providers and tooling depends on an annotation to be set on the `Machine`
to determine once it is done being provisioned. For example, the `clusterctl`
command does this [here](https://github.com/kubernetes-sigs/cluster-api/blob/a30de81123009a5f91ade870008c1a35f7ce4b35/cmd/clusterctl/clusterdeployer/clusterclient/clusterclient.go#L555):
```go
		// TODO: update once machine controllers have a way to indicate a machine has been provisoned. https://github.com/kubernetes-sigs/cluster-api/issues/253
		// Seeing a node cannot be purely relied upon because the provisioned master will not be registering with
		// the stack that provisions it.
		ready := m.Status.NodeRef != nil || len(m.Annotations) > 0
		return ready, nil
```

{% sample lang="go" %}
```go
// MachineSpec defines the desired state of Machine
type MachineSpec struct {
	// This ObjectMeta will autopopulate the Node created. Use this to
	// indicate what labels, annotations, name prefix, etc., should be used
	// when creating the Node.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The full, authoritative list of taints to apply to the corresponding
	// Node. This list will overwrite any modifications made to the Node on
	// an ongoing basis.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// Provider-specific configuration to use during node creation.
	// +optional
	ProviderSpec ProviderSpec `json:"providerSpec"`

	// Versions of key software to use. This field is optional at cluster
	// creation time, and omitting the field indicates that the cluster
	// installation tool should select defaults for the user. These
	// defaults may differ based on the cluster installer, but the tool
	// should populate the values it uses when persisting Machine objects.
	// A Machine spec missing this field at runtime is invalid.
	// +optional
	Versions MachineVersionInfo `json:"versions,omitempty"`

	// To populate in the associated Node for dynamic kubelet config. This
	// field already exists in Node, so any updates to it in the Machine
	// spec will be automatially copied to the linked NodeRef from the
	// status. The rest of dynamic kubelet config support should then work
	// as-is.
	// +optional
	ConfigSource *corev1.NodeConfigSource `json:"configSource,omitempty"`
}
```
{% endmethod %}

{% method %}
## MachineStatus

Note that the **NodeRef** field may not exist, in particular in the
case of providers which manage remote Kubernetes clusters.

{% sample lang="go" %}
```go
// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// If the corresponding Node exists, this will point to its object.
	// +optional
	NodeRef *corev1.ObjectReference `json:"nodeRef,omitempty"`

	// When was this status last observed
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// The current versions of software on the corresponding Node (if it
	// exists). This is provided for a few reasons:
	//
	// 1) It is more convenient than checking the NodeRef, traversing it to
	//    the Node, and finding the appropriate field in Node.Status.NodeInfo
	//    (which uses different field names and formatting).
	// 2) It removes some of the dependency on the structure of the Node,
	//    so that if the structure of Node.Status.NodeInfo changes, only
	//    machine controllers need to be updated, rather than every client
	//    of the Machines API.
	// 3) There is no other simple way to check the ControlPlane
	//    version. A client would have to connect directly to the apiserver
	//    running on the target node in order to find out its version.
	// +optional
	Versions *MachineVersionInfo `json:"versions,omitempty"`

	// In the event that there is a terminal problem reconciling the
	// Machine, both ErrorReason and ErrorMessage will be set. ErrorReason
	// will be populated with a succinct value suitable for machine
	// interpretation, while ErrorMessage will contain a more verbose
	// string suitable for logging and human consumption.
	//
	// These fields should not be set for transitive errors that a
	// controller faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	ErrorReason *common.MachineStatusError `json:"errorReason,omitempty"`
	// +optional
	ErrorMessage *string `json:"errorMessage,omitempty"`

	// Provider-specific status.
	// It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderStatus *runtime.RawExtension `json:"providerStatus,omitempty"`

	// Addresses is a list of addresses assigned to the machine. Queried from cloud provider, if available.
	// +optional
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`

	// List of conditions synced from the node conditions of the corresponding node-object.
	// Machine-controller is responsible for keeping conditions up-to-date.
	// MachineSet controller will be taking these conditions as a signal to decide if
	// machine is healthy or needs to be replaced.
	// Refer: https://kubernetes.io/docs/concepts/architecture/nodes/#condition
	// +optional
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
}
```
{% endmethod %}

{% method %}
## Machine Actuator Interface

All methods should be idempotent. Each time the Machine controller attempts
to reconcile the state it will call one or more of the following actuator
methods.

`Create()` will only be called when `Exists()` returns false.

`Update()` will only be called when `Exists()` returns true.

`Delete()` will only be called when the `Machine` is in the process of being
deleted.

The definition of `Exist()` is determined by the provider.

**TODO**: Provide more guidance on `Exists()`.

{% sample lang="go" %}
```go
// Actuator controls machines on a specific infrastructure. All
// methods should be idempotent unless otherwise specified.
type Actuator interface {
	// Create the machine.
	Create(*clusterv1.Cluster, *clusterv1.Machine) error
	// Delete the machine. If no error is returned, it is assumed that all dependent resources have been cleaned up.
	Delete(*clusterv1.Cluster, *clusterv1.Machine) error
	// Update the machine to the provided definition.
	Update(*clusterv1.Cluster, *clusterv1.Machine) error
	// Checks if the machine currently exists.
	Exists(*clusterv1.Cluster, *clusterv1.Machine) (bool, error)
}
```
{% endmethod %}

## Machine Controller Semantics

{% panel style="info", title="Logic sequence" %}
We need a diagrams tracing the logic from resource creation though updates
and finally deletion.
{% endpanel %}

0. Determine the `Cluster` associated with the `Machine`.
- If the `Machine` hasn't been deleted and doesn't have a finalizer, add one.
- If the `Machine` is being deleted, and there is no finalier, we're done
  - Check if the `Machine` is allowed to be deleted. [^1]
  - Call the provider specific actuators `Delete()` method.
    - If the `Delete()` method returns true, remove the finalizer.
- Check if the `Machine` exists by calling the provider specific `Exists()`
method.
  - If it does, call the `Update()` method.
  - If the `Update()` fails and returns a retryable error:
    - Retry the `Update()` after N seconds.
- If the machine does not exist, attempt to create machine by calling
  actuator `Create()` method.

{% panel style="warning", title="Machines depend on Clusters" %}
The Machine actuator methods expect both a `Cluster` and a `Machine` to be
passed in. While there is not a strong link between `Cluster`s and `Machines`,
the machine controller will determine which cluster to pass by looking for a 
`Cluster` in the same namespace as the `Machine`

There are two consequences of this:
 - There must be at exactly one `Cluster` per namespace.
 - If the `Cluster` is deleted before the `Machine` it will not be possible to
   delete the `Machine`. Therefore `Machines` must be deleted before `Cluster`s.
{% endpanel %}

---

[^1] One reason a `Machine` may not be deleted is if it corresponds to the 
node running the Machine controller.

[machine_types_source]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/apis/cluster/v1alpha1/machine_types.go
