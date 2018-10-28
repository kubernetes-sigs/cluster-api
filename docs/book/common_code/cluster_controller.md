
# Cluster Resources

A `Cluster` represents the global configuration of a Kubernetes cluster.

{% panel style="info", title="Important" %}
To see the actual and current definitions see the [source](#cluster_source).
{% endpanel %}

{% method %}
## Cluster

Cluster has 4 fields:

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
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}
```
{% endmethod %}

{% method %}
## ClusterSpec

The `ClusterNetwork` field includes the information necessary to configure
kubelet networking for `Pod`s and `Service`s.

{% sample lang="go" %}
```go
// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// Cluster network configuration
	ClusterNetwork ClusterNetworkingConfig `json:"clusterNetwork"`

	// Provider-specific serialized configuration to use during
	// cluster creation. It is recommended that providers maintain
	// their own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderSpec ProviderSpec `json:"providerSpec,omitempty"`
}
```
{% endmethod %}

{% method %}
## ClusterStatus

Some providers use the `APIEndpoint` field to determine when one or more
masters have been provisioned. This may be necessary before worker nodes
can be provisioned. For example, the `cluster-api-provider-gcp` does [this](
https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/f3145d8810a5c7fc434ddb5577699b4deb1b5fa6/pkg/cloud/google/metadata.go#L43):

```go
	if len(cluster.Status.APIEndpoints) == 0 {
		return nil, fmt.Errorf("master endpoint not found in apiEndpoints for cluster %v", cluster)
	}
```

**TODO**: Provide examples of how `ErrorReason` and `ErrorMessage` are 
used in practice.

{% sample lang="go" %}
```go
// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	// APIEndpoint represents the endpoint to communicate with the IP.
	// +optional
	APIEndpoints []APIEndpoint `json:"apiEndpoints,omitempty"`

	// NB: Eventually we will redefine ErrorReason as ClusterStatusError once the
	// following issue is fixed.
	// https://github.com/kubernetes-incubator/apiserver-builder/issues/176

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	ErrorReason common.ClusterStatusError `json:"errorReason,omitempty"`

	// If set, indicates that there is a problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// Provider-specific status.
	// It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderStatus *runtime.RawExtension `json:"providerStatus,omitempty"`
}
```
{% endmethod %}

{% method %}
## Cluster Actuator Interface

All methods should be idempotent.

`Reconcile()` will be called whenever there is a change to the `Cluster` Spec,
or after every resync period.

If a `Cluster` resource is deleted, the controller will call the actuators 
`Delete()` method until it succeeds, or the finallizer is removed (see 
below).

**TODO**: Determine what the current resync period is.

{% sample lang="go" %}
```go
// Actuator controls clusters on a specific infrastructure. All
// methods should be idempotent unless otherwise specified.
type Actuator interface {
	// Create or update the cluster
	Reconcile(*clusterv1.Cluster) error
	// Delete the cluster.
	Delete(*clusterv1.Cluster) error
}
```
{% endmethod %}

## Cluster Controller Semantics

{% panel style="info", title="Logic sequence" %}
We need a diagrams tracing the logic from resource creation though updates
and finally deletion. This was done using the sequences GitBook plugin.
Unfortunately there are (possibly personal) problems with phantomjs which
are making this difficult.
{% endpanel %}

0. If the `Cluster` hasn't been deleted and doesn't have a finalizer, add one.
- If the `Cluster` is being deleted, and there is no finalizer, we're done.
- Call the provider specific `Delete()` method.
  - If the `Delete()` method returns true, remove the finalizer, we're done.
- If the `Cluster` has not been deleted, called the `Reconcile()` method.

[cluster_source]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/apis/cluster/v1alpha1/cluster_types.go
