/*
Copyright 2019 The Kubernetes Authors.

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

// Package controllers implements controller functionality.
package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerClusterReconciler reconciles a DockerCluster object.
type DockerClusterReconciler struct {
	client.Client
	ContainerRuntime  container.Runtime
	backendReconciler *dockerbackend.ClusterBackEndReconciler

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters/status;dockerclusters/finalizers,verbs=get;list;watch;patch;update

// Reconcile reads that state of the cluster for a DockerCluster object and makes changes based on the state read
// and what is in the DockerCluster.Spec.
func (r *DockerClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerCluster instance
	dockerCluster := &infrav1.DockerCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, dockerCluster, infrav1.ClusterFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, dockerCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on DockerCluster")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, dockerCluster); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	devCluster := dockerClusterToDevCluster(dockerCluster)

	// Always attempt to Patch the DockerCluster object and status after each reconciliation.
	defer func() {
		devClusterToDockerCluster(devCluster, dockerCluster)
		if err := patchDockerCluster(ctx, patchHelper, dockerCluster); err != nil {
			log.Error(err, "Failed to patch DockerCluster")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Handle deleted clusters
	if !dockerCluster.DeletionTimestamp.IsZero() {
		return r.backendReconciler.ReconcileDelete(ctx, cluster, devCluster)
	}

	// Handle non-deleted clusters
	return r.backendReconciler.ReconcileNormal(ctx, cluster, devCluster)
}

// SetupWithManager will add watches for this controller.
func (r *DockerClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ContainerRuntime == nil {
		return errors.New("Client and ContainerRuntime must not be nil")
	}
	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "dockercluster")
	err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DockerCluster{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("DockerCluster"), mgr.GetClient(), &infrav1.DockerCluster{})),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ClusterPausedTransitions(mgr.GetScheme(), predicateLog),
			)),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.backendReconciler = &dockerbackend.ClusterBackEndReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		NewPatchHelperFunc: func(obj client.Object, crClient client.Client) (*patch.Helper, error) {
			devCluster, ok := obj.(*infrav1.DevCluster)
			if !ok {
				panic(fmt.Sprintf("Expected obj to be *infrav1.DevCluster, got %T", obj))
			}
			dockerCluster := &infrav1.DockerCluster{}
			devClusterToDockerCluster(devCluster, dockerCluster)
			return patch.NewHelper(dockerCluster, crClient)
		},
		PatchDevClusterFunc: func(ctx context.Context, patchHelper *patch.Helper, devCluster *infrav1.DevCluster) error {
			dockerCluster := &infrav1.DockerCluster{}
			devClusterToDockerCluster(devCluster, dockerCluster)
			return patchDockerCluster(ctx, patchHelper, dockerCluster)
		},
	}

	return nil
}

func patchDockerCluster(ctx context.Context, patchHelper *patch.Helper, dockerCluster *infrav1.DockerCluster) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	conditions.SetSummary(dockerCluster,
		conditions.WithConditions(
			infrav1.LoadBalancerAvailableCondition,
		),
		conditions.WithStepCounterIf(dockerCluster.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerCluster,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.LoadBalancerAvailableCondition,
		}},
	)
}

func dockerClusterToDevCluster(dockerCluster *infrav1.DockerCluster) *infrav1.DevCluster {
	var v1Beta2Status *infrav1.DevClusterV1Beta2Status
	if dockerCluster.Status.V1Beta2 != nil {
		v1Beta2Status = &infrav1.DevClusterV1Beta2Status{
			Conditions: dockerCluster.Status.V1Beta2.Conditions,
		}
	}

	return &infrav1.DevCluster{
		ObjectMeta: dockerCluster.ObjectMeta,
		Spec: infrav1.DevClusterSpec{
			ControlPlaneEndpoint: dockerCluster.Spec.ControlPlaneEndpoint,
			Backend: infrav1.DevClusterBackendSpec{
				Docker: &infrav1.DockerClusterBackendSpec{
					FailureDomains: dockerCluster.Spec.FailureDomains,
					LoadBalancer:   dockerCluster.Spec.LoadBalancer,
				},
			},
		},
		Status: infrav1.DevClusterStatus{
			Ready:          dockerCluster.Status.Ready,
			FailureDomains: dockerCluster.Status.FailureDomains,
			Conditions:     dockerCluster.Status.Conditions,
			V1Beta2:        v1Beta2Status,
		},
	}
}

func devClusterToDockerCluster(devCluster *infrav1.DevCluster, dockerCluster *infrav1.DockerCluster) {
	var v1Beta2Status *infrav1.DockerClusterV1Beta2Status
	if devCluster.Status.V1Beta2 != nil {
		v1Beta2Status = &infrav1.DockerClusterV1Beta2Status{
			Conditions: devCluster.Status.V1Beta2.Conditions,
		}
	}

	dockerCluster.ObjectMeta = devCluster.ObjectMeta
	dockerCluster.Spec.ControlPlaneEndpoint = devCluster.Spec.ControlPlaneEndpoint
	dockerCluster.Spec.FailureDomains = devCluster.Spec.Backend.Docker.FailureDomains
	dockerCluster.Spec.LoadBalancer = devCluster.Spec.Backend.Docker.LoadBalancer
	dockerCluster.Status.Ready = devCluster.Status.Ready
	dockerCluster.Status.FailureDomains = devCluster.Status.FailureDomains
	dockerCluster.Status.Conditions = devCluster.Status.Conditions
	dockerCluster.Status.V1Beta2 = v1Beta2Status
}
