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

package controllers

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// deleteRequeueAfter is how long to wait before checking again to see if the cluster still has children during
	// deletion.
	deleteRequeueAfter = 5 * time.Second
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;clusters/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// ClusterReconciler reconciles a Cluster object.
type ClusterReconciler struct {
	Client client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

func (r *ClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(r.controlPlaneMachineToCluster),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("cluster-controller")
	r.externalTracker = external.ObjectTracker{
		Controller: controller,
	}
	return nil
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, cluster) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(cluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always reconcile the Status.Phase field.
		r.reconcilePhase(ctx, cluster)

		// Always attempt to Patch the Cluster object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchCluster(ctx, patchHelper, cluster, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(cluster, clusterv1.ClusterFinalizer) {
		controllerutil.AddFinalizer(cluster, clusterv1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deletion reconciliation loop.
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, cluster)
}

func patchCluster(ctx context.Context, patchHelper *patch.Helper, cluster *clusterv1.Cluster, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(cluster,
		conditions.WithConditions(
			clusterv1.ControlPlaneReadyCondition,
			clusterv1.InfrastructureReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	// Also, if requested, we are adding additional options like e.g. Patch ObservedGeneration when issuing the
	// patch at the end of the reconcile loop.
	options = append(options,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.ControlPlaneReadyCondition,
			clusterv1.InfrastructureReadyCondition,
		}},
	)
	return patchHelper.Patch(ctx, cluster, options...)
}

// reconcile handles cluster reconciliation.
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx, "cluster", cluster.Name)

	if cluster.Spec.Topology != nil {
		if cluster.Spec.ControlPlaneRef == nil || cluster.Spec.InfrastructureRef == nil {
			// TODO: add a condition to surface this scenario
			log.Info("Waiting for the topology to be generated")
			return ctrl.Result{}, nil
		}
	}

	phases := []func(context.Context, *clusterv1.Cluster) (ctrl.Result, error){
		r.reconcileInfrastructure,
		r.reconcileControlPlane,
		r.reconcileKubeconfig,
		r.reconcileControlPlaneInitialized,
	}

	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, cluster)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}
	return res, kerrors.NewAggregate(errs)
}

// reconcileDelete handles cluster deletion.
func (r *ClusterReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	descendants, err := r.listDescendants(ctx, cluster)
	if err != nil {
		log.Error(err, "Failed to list descendants")
		return reconcile.Result{}, err
	}

	children, err := descendants.filterOwnedDescendants(cluster)
	if err != nil {
		log.Error(err, "Failed to extract direct descendants")
		return reconcile.Result{}, err
	}

	if len(children) > 0 {
		log.Info("Cluster still has children - deleting them first", "count", len(children))

		var errs []error

		for _, child := range children {
			if !child.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}
			gvk := child.GetObjectKind().GroupVersionKind().String()

			log.Info("Deleting child object", "gvk", gvk, "name", child.GetName())
			if err := r.Client.Delete(ctx, child); err != nil {
				err = errors.Wrapf(err, "error deleting cluster %s/%s: failed to delete %s %s", cluster.Namespace, cluster.Name, gvk, child.GetName())
				log.Error(err, "Error deleting resource", "gvk", gvk, "name", child.GetName())
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errs)
		}
	}

	if descendantCount := descendants.length(); descendantCount > 0 {
		indirect := descendantCount - len(children)
		log.Info("Cluster still has descendants - need to requeue", "descendants", descendants.descendantNames(), "indirect descendants count", indirect)
		// Requeue so we can check the next time to see if there are still any descendants left.
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	if cluster.Spec.ControlPlaneRef != nil {
		obj, err := external.Get(ctx, r.Client, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		switch {
		case apierrors.IsNotFound(errors.Cause(err)):
			// All good - the control plane resource has been deleted
			conditions.MarkFalse(cluster, clusterv1.ControlPlaneReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		case err != nil:
			return reconcile.Result{}, errors.Wrapf(err, "failed to get %s %q for Cluster %s/%s",
				path.Join(cluster.Spec.ControlPlaneRef.APIVersion, cluster.Spec.ControlPlaneRef.Kind),
				cluster.Spec.ControlPlaneRef.Name, cluster.Namespace, cluster.Name)
		default:
			// Report a summary of current status of the control plane object defined for this cluster.
			conditions.SetMirror(cluster, clusterv1.ControlPlaneReadyCondition,
				conditions.UnstructuredGetter(obj),
				conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, ""),
			)

			// Issue a deletion request for the control plane object.
			// Once it's been deleted, the cluster will get processed again.
			if err := r.Client.Delete(ctx, obj); err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed to delete %v %q for Cluster %q in namespace %q",
					obj.GroupVersionKind(), obj.GetName(), cluster.Name, cluster.Namespace)
			}

			// Return here so we don't remove the finalizer yet.
			log.Info("Cluster still has descendants - need to requeue", "controlPlaneRef", cluster.Spec.ControlPlaneRef.Name)
			return ctrl.Result{}, nil
		}
	}

	if cluster.Spec.InfrastructureRef != nil {
		obj, err := external.Get(ctx, r.Client, cluster.Spec.InfrastructureRef, cluster.Namespace)
		switch {
		case apierrors.IsNotFound(errors.Cause(err)):
			// All good - the infra resource has been deleted
			conditions.MarkFalse(cluster, clusterv1.InfrastructureReadyCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
		case err != nil:
			return ctrl.Result{}, errors.Wrapf(err, "failed to get %s %q for Cluster %s/%s",
				path.Join(cluster.Spec.InfrastructureRef.APIVersion, cluster.Spec.InfrastructureRef.Kind),
				cluster.Spec.InfrastructureRef.Name, cluster.Namespace, cluster.Name)
		default:
			// Report a summary of current status of the infrastructure object defined for this cluster.
			conditions.SetMirror(cluster, clusterv1.InfrastructureReadyCondition,
				conditions.UnstructuredGetter(obj),
				conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, ""),
			)

			// Issue a deletion request for the infrastructure object.
			// Once it's been deleted, the cluster will get processed again.
			if err := r.Client.Delete(ctx, obj); err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed to delete %v %q for Cluster %q in namespace %q",
					obj.GroupVersionKind(), obj.GetName(), cluster.Name, cluster.Namespace)
			}

			// Return here so we don't remove the finalizer yet.
			log.Info("Cluster still has descendants - need to requeue", "infrastructureRef", cluster.Spec.InfrastructureRef.Name)
			return ctrl.Result{}, nil
		}
	}

	controllerutil.RemoveFinalizer(cluster, clusterv1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

type clusterDescendants struct {
	machineDeployments   clusterv1.MachineDeploymentList
	machineSets          clusterv1.MachineSetList
	controlPlaneMachines clusterv1.MachineList
	workerMachines       clusterv1.MachineList
	machinePools         expv1.MachinePoolList
}

// length returns the number of descendants.
func (c *clusterDescendants) length() int {
	return len(c.machineDeployments.Items) +
		len(c.machineSets.Items) +
		len(c.controlPlaneMachines.Items) +
		len(c.workerMachines.Items) +
		len(c.machinePools.Items)
}

func (c *clusterDescendants) descendantNames() string {
	descendants := make([]string, 0)
	controlPlaneMachineNames := make([]string, len(c.controlPlaneMachines.Items))
	for i, controlPlaneMachine := range c.controlPlaneMachines.Items {
		controlPlaneMachineNames[i] = controlPlaneMachine.Name
	}
	if len(controlPlaneMachineNames) > 0 {
		descendants = append(descendants, "Control plane machines: "+strings.Join(controlPlaneMachineNames, ","))
	}
	machineDeploymentNames := make([]string, len(c.machineDeployments.Items))
	for i, machineDeployment := range c.machineDeployments.Items {
		machineDeploymentNames[i] = machineDeployment.Name
	}
	if len(machineDeploymentNames) > 0 {
		descendants = append(descendants, "Machine deployments: "+strings.Join(machineDeploymentNames, ","))
	}
	machineSetNames := make([]string, len(c.machineSets.Items))
	for i, machineSet := range c.machineSets.Items {
		machineSetNames[i] = machineSet.Name
	}
	if len(machineSetNames) > 0 {
		descendants = append(descendants, "Machine sets: "+strings.Join(machineSetNames, ","))
	}
	workerMachineNames := make([]string, len(c.workerMachines.Items))
	for i, workerMachine := range c.workerMachines.Items {
		workerMachineNames[i] = workerMachine.Name
	}
	if len(workerMachineNames) > 0 {
		descendants = append(descendants, "Worker machines: "+strings.Join(workerMachineNames, ","))
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		machinePoolNames := make([]string, len(c.machinePools.Items))
		for i, machinePool := range c.machinePools.Items {
			machinePoolNames[i] = machinePool.Name
		}
		if len(machinePoolNames) > 0 {
			descendants = append(descendants, "Machine pools: "+strings.Join(machinePoolNames, ","))
		}
	}
	return strings.Join(descendants, ";")
}

// listDescendants returns a list of all MachineDeployments, MachineSets, MachinePools and Machines for the cluster.
func (r *ClusterReconciler) listDescendants(ctx context.Context, cluster *clusterv1.Cluster) (clusterDescendants, error) {
	var descendants clusterDescendants

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{clusterv1.ClusterLabelName: cluster.Name}),
	}

	if err := r.Client.List(ctx, &descendants.machineDeployments, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list MachineDeployments for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if err := r.Client.List(ctx, &descendants.machineSets, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list MachineSets for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		if err := r.Client.List(ctx, &descendants.machinePools, listOptions...); err != nil {
			return descendants, errors.Wrapf(err, "failed to list MachinePools for the cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}
	var machines clusterv1.MachineList
	if err := r.Client.List(ctx, &machines, listOptions...); err != nil {
		return descendants, errors.Wrapf(err, "failed to list Machines for cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	// Split machines into control plane and worker machines so we make sure we delete control plane machines last
	machineCollection := collections.FromMachineList(&machines)
	controlPlaneMachines := machineCollection.Filter(collections.ControlPlaneMachines(cluster.Name))
	workerMachines := machineCollection.Difference(controlPlaneMachines)
	descendants.workerMachines = collections.ToMachineList(workerMachines)
	// Only count control plane machines as descendants if there is no control plane provider.
	if cluster.Spec.ControlPlaneRef == nil {
		descendants.controlPlaneMachines = collections.ToMachineList(controlPlaneMachines)
	}

	return descendants, nil
}

// filterOwnedDescendants returns an array of runtime.Objects containing only those descendants that have the cluster
// as an owner reference, with control plane machines sorted last.
func (c clusterDescendants) filterOwnedDescendants(cluster *clusterv1.Cluster) ([]client.Object, error) {
	var ownedDescendants []client.Object
	eachFunc := func(o runtime.Object) error {
		obj := o.(client.Object)
		acc, err := meta.Accessor(obj)
		if err != nil {
			return nil //nolint:nilerr // We don't want to exit the EachListItem loop, just continue
		}

		if util.IsOwnedByObject(acc, cluster) {
			ownedDescendants = append(ownedDescendants, obj)
		}

		return nil
	}

	lists := []client.ObjectList{
		&c.machineDeployments,
		&c.machineSets,
		&c.workerMachines,
		&c.controlPlaneMachines,
	}
	if feature.Gates.Enabled(feature.MachinePool) {
		lists = append([]client.ObjectList{&c.machinePools}, lists...)
	}

	for _, list := range lists {
		if err := meta.EachListItem(list, eachFunc); err != nil {
			return nil, errors.Wrapf(err, "error finding owned descendants of cluster %s/%s", cluster.Namespace, cluster.Name)
		}
	}

	return ownedDescendants, nil
}

func (r *ClusterReconciler) reconcileControlPlaneInitialized(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Skip checking if the control plane is initialized when using a Control Plane Provider (this is reconciled in
	// reconcileControlPlane instead).
	if cluster.Spec.ControlPlaneRef != nil {
		log.V(4).Info("Skipping reconcileControlPlaneInitialized because cluster has a controlPlaneRef")
		return ctrl.Result{}, nil
	}

	if conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		log.V(4).Info("Skipping reconcileControlPlaneInitialized because control plane already initialized")
		return ctrl.Result{}, nil
	}

	log.V(4).Info("Checking for control plane initialization")

	machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, cluster, collections.ActiveMachines)
	if err != nil {
		log.Error(err, "unable to determine ControlPlaneInitialized")
		return ctrl.Result{}, err
	}

	for _, m := range machines {
		if util.IsControlPlaneMachine(m) && m.Status.NodeRef != nil {
			conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedCondition)
			return ctrl.Result{}, nil
		}
	}

	conditions.MarkFalse(cluster, clusterv1.ControlPlaneInitializedCondition, clusterv1.MissingNodeRefReason, clusterv1.ConditionSeverityInfo, "Waiting for the first control plane machine to have its status.nodeRef set")

	return ctrl.Result{}, nil
}

// controlPlaneMachineToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update its status.controlPlaneInitialized field.
func (r *ClusterReconciler) controlPlaneMachineToCluster(o client.Object) []ctrl.Request {
	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}
	if !util.IsControlPlaneMachine(m) {
		return nil
	}
	if m.Status.NodeRef == nil {
		return nil
	}

	cluster, err := util.GetClusterByName(context.TODO(), r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		return nil
	}

	if conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: util.ObjectKey(cluster),
	}}
}
