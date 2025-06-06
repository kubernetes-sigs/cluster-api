/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package inmemory implements in memory backend for DevClusters and DevMachines.
package inmemory

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ClusterBackendReconciler reconciles a InMemoryCluster backend.
type ClusterBackendReconciler struct {
	client.Client
	InMemoryManager inmemoryruntime.Manager
	APIServerMux    *inmemoryserver.WorkloadClustersMux
}

// HotRestart tries to setup the APIServerMux according to an existing sets of InMemoryCluster.
// NOTE: This is done at best effort in order to make iterative development workflow easier.
func (r *ClusterBackendReconciler) HotRestart(ctx context.Context) error {
	inMemoryClusterList := &infrav1.DevClusterList{}
	if err := r.List(ctx, inMemoryClusterList); err != nil {
		return err
	}

	listeners := []inmemoryserver.HotRestartListener{}
	for _, cluster := range inMemoryClusterList.Items {
		if cluster.Spec.Backend.InMemory != nil {
			listeners = append(listeners, inmemoryserver.HotRestartListener{
				Cluster: klog.KRef(cluster.Namespace, cluster.Name).String(),
				Name:    cluster.Annotations[infrav1.ListenerAnnotationName],
				Host:    cluster.Spec.ControlPlaneEndpoint.Host,
				Port:    cluster.Spec.ControlPlaneEndpoint.Port,
			})
		}
	}
	return r.APIServerMux.HotRestart(listeners)
}

// ReconcileNormal handle in memory backend for DevCluster not yet deleted.
func (r *ClusterBackendReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.DevCluster) (ctrl.Result, error) {
	if inMemoryCluster.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevClusters without an InMemory backend")
	}

	// Compute the name for resource group and listener.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()

	// Store the resource group used by this inMemoryCluster.
	inMemoryCluster.Annotations[infrav1.ListenerAnnotationName] = listenerName

	// Create a resource group for all the in memory resources belonging the workload cluster;
	// if the resource group already exists, the operation is a no-op.
	// NOTE: We are storing in this resource group both the in memory resources (e.g. VM) as
	// well as Kubernetes resources that are expected to exist on the workload cluster (e.g Nodes).
	r.InMemoryManager.AddResourceGroup(resourceGroup)

	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create default Namespaces.
	for _, nsName := range []string{metav1.NamespaceDefault, metav1.NamespacePublic, metav1.NamespaceSystem} {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"kubernetes.io/metadata.name": nsName,
				},
			},
		}

		if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(ns), ns); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to get %s Namespace", nsName)
			}

			if err := inmemoryClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to create %s Namespace", nsName)
			}
		}
	}

	// Initialize a listener for the workload cluster; if the listener has been already initialized
	// the operation is a no-op.
	listener, err := r.APIServerMux.InitWorkloadClusterListener(listenerName)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to init the listener for the workload cluster")
	}
	if err := r.APIServerMux.RegisterResourceGroup(listenerName, resourceGroup); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to register the resource group for the workload cluster")
	}

	// Surface the control plane endpoint
	if inMemoryCluster.Spec.ControlPlaneEndpoint.Host == "" {
		inMemoryCluster.Spec.ControlPlaneEndpoint.Host = listener.Host()
		inMemoryCluster.Spec.ControlPlaneEndpoint.Port = listener.Port()
	}

	// Mark the InMemoryCluster ready
	inMemoryCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

// ReconcileDelete handle in memory backend for deleted DevCluster.
func (r *ClusterBackendReconciler) ReconcileDelete(_ context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.DevCluster) (ctrl.Result, error) {
	if inMemoryCluster.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevClusters without an InMemory backend")
	}

	// Compute the name for resource group and listener.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()

	// Delete the resource group hosting all the in memory resources belonging the workload cluster;
	r.InMemoryManager.DeleteResourceGroup(resourceGroup)

	// Delete the listener for the workload cluster;
	if err := r.APIServerMux.DeleteWorkloadClusterListener(listenerName); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(inMemoryCluster, infrav1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

// PatchDevCluster patch a DevCluster.
func (r *ClusterBackendReconciler) PatchDevCluster(ctx context.Context, patchHelper *patch.Helper, inMemoryCluster *infrav1.DevCluster) error {
	if inMemoryCluster.Spec.Backend.InMemory == nil {
		return errors.New("InMemoryBackendReconciler can't be called for DevClusters without an InMemory backend")
	}

	return patchHelper.Patch(
		ctx,
		inMemoryCluster,
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
		}},
	)
}
