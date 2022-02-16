/*
Copyright 2020 The Kubernetes Authors.

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
	"strings"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachinePoolReconciler reconciles a DockerMachinePool object.
type DockerMachinePoolReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ContainerRuntime container.Runtime
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools/status;dockermachinepools/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepoolmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepoolmachines/status;dockermachinepoolmachines/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *DockerMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerMachinePool instance.
	dockerMachinePool := &infraexpv1.DockerMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachinePool); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Machinepool not found, returning")
		}
		return ctrl.Result{}, err
	}

	// Fetch the MachinePool.
	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, dockerMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on DockerMachinePool")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("machine-pool", machinePool.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("DockerMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterLabelName))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the DockerMachinePool object and status after each reconciliation.
	defer func() {
		if err := patchDockerMachinePool(ctx, patchHelper, dockerMachinePool); err != nil {
			log.Error(err, "failed to patch DockerMachinePool")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer) {
		controllerutil.AddFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	log.Info(fmt.Sprintf("Deletion timestamp is %+v", dockerMachinePool.DeletionTimestamp))
	log.Info("Is zero? ", "bool", dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero())
	if !dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, machinePool, dockerMachinePool)
	}

	// Handle non-deleted machines
	log.Info("Reconciling machinepool normally")
	return r.reconcileNormal(ctx, cluster, machinePool, dockerMachinePool)
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToDockerMachinePools, err := util.ClusterToObjectsMapper(mgr.GetClient(), &infraexpv1.DockerMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infraexpv1.DockerMachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(
			&source.Kind{Type: &expv1.MachinePool{}},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(
				infraexpv1.GroupVersion.WithKind("DockerMachinePool"), ctrl.LoggerFrom(ctx))),
		).
		Build(r)
	if err != nil {
		return err
	}
	return c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(clusterToDockerMachinePools),
		predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
	)
}

func (r *DockerMachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (ctrl.Result, error) {
	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build new node pool")
	}

	if err := pool.Delete(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to delete all machines in the node pool")
	}

	controllerutil.RemoveFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
	return ctrl.Result{}, nil
}

func (r *DockerMachinePoolReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Make sure bootstrap data is available and populated.
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	if machinePool.Spec.Replicas == nil {
		machinePool.Spec.Replicas = pointer.Int32Ptr(1)
	}

	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build new node pool")
	}

	// Reconcile machines and updates Status.Instances
	if res, err := pool.ReconcileMachines(ctx); err != nil || !res.IsZero() {
		return res, err
	}

	// Derive info from Status.Instances
	providerIDList := make([]string, len(dockerMachinePool.Status.Instances))
	infraRefList := make([]corev1.ObjectReference, len(dockerMachinePool.Status.Instances))
	for i, instance := range dockerMachinePool.Status.Instances {
		if instance.ProviderID != nil {
			name := strings.TrimPrefix(*instance.ProviderID, "docker:////")
			infraRefList[i] = corev1.ObjectReference{
				Kind:       "DockerMachinePoolMachine",
				Name:       name,
				Namespace:  dockerMachinePool.Namespace,
				APIVersion: infraexpv1.GroupVersion.String(),
			}
			if instance.Ready {
				providerIDList[i] = *instance.ProviderID
			}

			// Look up the DockerMachinePoolMachine object.
			dmpm := &infraexpv1.DockerMachinePoolMachine{}
			if err := r.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: dockerMachinePool.Namespace}, dmpm); err != nil {
				// TODO: check the error to see that it was a 404?
				// Create the DockerMachinePoolMachine object if needed.
				dmpm := &infraexpv1.DockerMachinePoolMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: dockerMachinePool.Namespace,
					},
					Spec: infraexpv1.DockerMachinePoolMachineSpec{
						ProviderID: instance.ProviderID,
					},
				}
				// Find the corresponding Machine and set it as the OwnerReference
				m := &clusterv1.Machine{}
				if err := r.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: dockerMachinePool.Namespace}, m); err != nil {
					continue
				}
				controllerutil.SetControllerReference(m, dmpm, r.Client.Scheme())
				log.Info("Creating dmpm", "name", dmpm.Name, "namespace", dmpm.Namespace)
				err = r.Client.Create(ctx, dmpm)
				if err != nil {
					return ctrl.Result{}, errors.Wrap(err, "failed to create DockerMachinePoolMachine")
				}
			}
			// Update the DockerMachinePoolMachine object's status.
			if instance.Ready {
				dmpm.Status.Ready = true
				r.Client.Status().Update(ctx, dmpm)
			}
		}
	}

	dockerMachinePool.Spec.ProviderIDList = providerIDList
	dockerMachinePool.Spec.InfrastructureRefList = infraRefList

	dockerMachinePool.Status.Replicas = int32(len(dockerMachinePool.Status.Instances))

	if dockerMachinePool.Spec.ProviderID == "" {
		// This is a fake provider ID which does not tie back to any docker infrastructure. In cloud providers,
		// this ID would tie back to the resource which manages the machine pool implementation. For example,
		// Azure uses a VirtualMachineScaleSet to manage a set of like machines.
		dockerMachinePool.Spec.ProviderID = getDockerMachinePoolProviderID(cluster.Name, dockerMachinePool.Name)
	}

	dockerMachinePool.Status.Ready = len(dockerMachinePool.Spec.ProviderIDList) == int(*machinePool.Spec.Replicas)
	if dockerMachinePool.Status.Ready {
		conditions.MarkTrue(dockerMachinePool, expv1.ReplicasReadyCondition)
	} else {
		conditions.MarkFalse(dockerMachinePool, expv1.ReplicasReadyCondition, expv1.WaitingForReplicasReadyReason, clusterv1.ConditionSeverityInfo, "")
		// TODO: is this requeue necessary?
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func getDockerMachinePoolProviderID(clusterName, dockerMachinePoolName string) string {
	return fmt.Sprintf("docker:////%s-dmp-%s", clusterName, dockerMachinePoolName)
}

func patchDockerMachinePool(ctx context.Context, patchHelper *patch.Helper, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	conditions.SetSummary(dockerMachinePool,
		conditions.WithConditions(
			expv1.ReplicasReadyCondition,
		),
	)

	return patchHelper.Patch(
		ctx,
		dockerMachinePool,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			expv1.ReplicasReadyCondition,
		}},
	)
}
