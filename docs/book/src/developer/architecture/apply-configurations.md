# Using ApplyConfigurations for Server-Side Apply

Cluster API provides generated ApplyConfiguration types that enable type-safe Server-Side Apply (SSA) operations in Go. This allows multiple controllers and operators to cooperatively manage Cluster API resources without conflicts.

## Overview

ApplyConfigurations provide a builder pattern for constructing partial resource definitions that can be applied using Kubernetes Server-Side Apply. Unlike traditional Update operations, SSA allows multiple actors (controllers, operators, users) to manage different fields of the same resource independently.

## Benefits of Server-Side Apply

- **Field Management**: Kubernetes tracks which controller owns which fields
- **Conflict Resolution**: Automatic conflict detection and resolution
- **Cooperative Management**: Multiple controllers can safely update different fields
- **Atomic Operations**: Changes are applied atomically

## Generated Packages

The generated code is located in:
- **ApplyConfigurations**: `pkg/generated/applyconfiguration/`
- **Typed Clients**: `pkg/generated/client/`

Supported API groups:
- `core/v1beta2` - Cluster, Machine, MachineDeployment, MachineSet, MachinePool, ClusterClass, MachineHealthCheck
- `addons/v1beta2` - ClusterResourceSet, ClusterResourceSetBinding
- `bootstrap/kubeadm/v1beta2` - KubeadmConfig
- `controlplane/kubeadm/v1beta2` - KubeadmControlPlane

## Usage Examples

### Creating a Cluster with SSA

```go
package main

import (
	"context"
	
	applycorev1beta2 "sigs.k8s.io/cluster-api/pkg/generated/applyconfiguration/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createCluster(ctx context.Context, c client.Client) error {
	// Build the Cluster apply configuration
	clusterApply := applycorev1beta2.Cluster("my-cluster", "default").
		WithSpec(applycorev1beta2.ClusterSpec().
			WithControlPlaneEndpoint(applycorev1beta2.APIEndpoint().
				WithHost("10.0.0.1").
				WithPort(6443)).
			WithClusterNetwork(applycorev1beta2.ClusterNetwork().
				WithServiceDomain("cluster.local")))
	
	// Apply the configuration
	err := c.Apply(ctx, clusterApply, 
		client.FieldOwner("my-controller"),
		client.ForceOwnership)
	
	return err
}
```

### Updating Machine Deployment Replicas

```go
func scaleDeployment(ctx context.Context, c client.Client, name, namespace string, replicas int32) error {
	// Build a partial apply configuration with just the replica count
	mdApply := applycorev1beta2.MachineDeployment(name, namespace).
		WithSpec(applycorev1beta2.MachineDeploymentSpec().
			WithReplicas(replicas))
	
	// Apply only the replica field
	return c.Apply(ctx, mdApply,
		client.FieldOwner("autoscaler"),
		client.ForceOwnership)
}
```

### Setting Infrastructure Reference

```go
func setInfrastructureRef(ctx context.Context, c client.Client, clusterName, namespace string) error {
	clusterApply := applycorev1beta2.Cluster(clusterName, namespace).
		WithSpec(applycorev1beta2.ClusterSpec().
			WithInfrastructureRef(
				&corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "MyInfraCluster",
					Name:       clusterName + "-infra",
					Namespace:  namespace,
				},
			))
	
	return c.Apply(ctx, clusterApply,
		client.FieldOwner("infrastructure-controller"))
}
```

### Working with Status Subresources

```go
func updateClusterStatus(ctx context.Context, c client.Client, clusterName, namespace string) error {
	statusApply := applycorev1beta2.Cluster(clusterName, namespace).
		WithStatus(applycorev1beta2.ClusterStatus().
			WithPhase("Provisioned").
			WithConditions(metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "ClusterReady",
			}))
	
	// Use Status().Apply() for status subresource
	return c.Status().Apply(ctx, statusApply,
		client.FieldOwner("cluster-controller"))
}
```

## Field Ownership

Each controller should use a unique `FieldOwner` identifier when calling Apply:

```go
client.FieldOwner("cluster-topology-controller")
client.FieldOwner("machine-controller")
client.FieldOwner("my-custom-operator")
```

This allows Kubernetes to track which controller manages which fields and detect conflicts.

## Force Ownership

Use `client.ForceOwnership` to take over fields from other controllers:

```go
err := c.Apply(ctx, clusterApply,
	client.FieldOwner("priority-controller"),
	client.ForceOwnership)  // Take over conflicting fields
```

**Warning**: Use `ForceOwnership` carefully as it can overwrite changes made by other controllers.

## Using the Typed Client

The generated typed client provides direct access to Apply methods:

```go
import (
	"k8s.io/client-go/rest"
	clientset "sigs.k8s.io/cluster-api/pkg/generated/client/clientset"
)

func example(config *rest.Config) error {
	// Create the clientset
	cs, err := clientset.NewForConfig(config)
	if err != nil {
		return err
	}
	
	// Use the typed client for core/v1beta2 resources
	clusterClient := cs.ClusterV1beta2().Clusters("default")
	
	// Apply configuration
	result, err := clusterClient.Apply(ctx, clusterApply, metav1.ApplyOptions{
		FieldManager: "my-controller",
	})
	
	return err
}
```

## Comparison with controller-runtime

### Using controller-runtime (existing approach)

```go
// Convert to Unstructured at runtime
modifiedUnstructured, err := prepareModified(c.Scheme(), modified)
if err != nil {
	return err
}

err = c.Apply(ctx, client.ApplyConfigurationFromUnstructured(modifiedUnstructured), 
	client.FieldOwner("my-controller"))
```

### Using ApplyConfigurations (new approach)

```go
// Type-safe builder pattern
clusterApply := applycorev1beta2.Cluster("my-cluster", "default").
	WithSpec(applycorev1beta2.ClusterSpec().
		WithControlPlaneEndpoint(...))

err := c.Apply(ctx, clusterApply,
	client.FieldOwner("my-controller"))
```

## Best Practices

1. **Use Specific Field Owners**: Choose descriptive, unique field owner names
2. **Minimize Force Ownership**: Only use when absolutely necessary
3. **Apply Partial Updates**: Only include fields you want to manage
4. **Handle Conflicts**: Check for and handle ownership conflicts gracefully
5. **Document Field Ownership**: Clearly document which controller owns which fields

## Limitations

- Only available for v1beta2 APIs
- Requires controller-runtime v0.18.0 or later
- Not all nested types may have apply configurations generated

## Troubleshooting

### Conflict Errors

If you encounter conflicts:
```
Apply failed: field managed by another controller
```

Solutions:
1. Use a different field or coordinate with the other controller
2. Use `ForceOwnership` if your controller should take precedence
3. Check the field manager using `kubectl get <resource> -o yaml` and look for `managedFields`

### Missing Apply Methods

If apply methods are missing for a type:
1. Verify the type has `+genclient` tags in its source
2. Run `make generate` to regenerate code
3. Check that the type is in a supported API group

## Additional Resources

- [Kubernetes Server-Side Apply Documentation](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- [Controller-Runtime Apply Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client#Client.Apply)
- [Field Management](https://kubernetes.io/docs/reference/using-api/server-side-apply/#field-management)
