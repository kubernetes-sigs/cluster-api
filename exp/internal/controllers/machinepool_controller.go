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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status;machinepools/finalizers,verbs=get;list;watch;create;update;patch;delete

var (
	// machinePoolKind contains the schema.GroupVersionKind for the MachinePool type.
	machinePoolKind = clusterv1.GroupVersion.WithKind("MachinePool")
)

const (
	// MachinePoolControllerName defines the controller used when creating clients.
	MachinePoolControllerName = "machinepool-controller"
)

// MachinePoolReconciler reconciles a MachinePool object.
type MachinePoolReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	controller      controller.Controller
	ssaCache        ssa.Cache
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

func (r *MachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &expv1.MachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&expv1.MachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachinePools),
			// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
			builder.WithPredicates(
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
					predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
				),
			),
		).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("machinepool-controller")
	r.externalTracker = external.ObjectTracker{
		Controller: c,
		Cache:      mgr.GetCache(),
	}
	r.ssaCache = ssa.NewCache()

	return nil
}

func (r *MachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	mp := &expv1.MachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, mp); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		log.Error(err, "Error reading the object - requeue the request.")
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(mp.ObjectMeta.Namespace, mp.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterByName(ctx, r.Client, mp.ObjectMeta.Namespace, mp.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get Cluster for MachinePool.", "MachinePool", klog.KObj(mp), "Cluster", klog.KRef(mp.ObjectMeta.Namespace, mp.Spec.ClusterName))
		return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster %q for machinepool %q in namespace %q",
			mp.Spec.ClusterName, mp.Name, mp.Namespace)
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, mp) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(mp, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		r.reconcilePhase(mp)
		// TODO(jpang): add support for metrics.

		// Always update the readyCondition with the summary of the machinepool conditions.
		conditions.SetSummary(mp,
			conditions.WithConditions(
				clusterv1.BootstrapReadyCondition,
				clusterv1.InfrastructureReadyCondition,
				expv1.ReplicasReadyCondition,
			),
		)

		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ReadyCondition,
				clusterv1.BootstrapReadyCondition,
				clusterv1.InfrastructureReadyCondition,
				expv1.ReplicasReadyCondition,
			}},
		}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, mp, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Reconcile labels.
	if mp.Labels == nil {
		mp.Labels = make(map[string]string)
	}
	mp.Labels[clusterv1.ClusterNameLabel] = mp.Spec.ClusterName

	// Handle deletion reconciliation loop.
	if !mp.ObjectMeta.DeletionTimestamp.IsZero() {
		err := r.reconcileDelete(ctx, cluster, mp)
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(mp, expv1.MachinePoolFinalizer) {
		controllerutil.AddFinalizer(mp, expv1.MachinePoolFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle normal reconciliation loop.
	res, err := r.reconcile(ctx, cluster, mp)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return res, err
}

func (r *MachinePoolReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, mp *expv1.MachinePool) (ctrl.Result, error) {
	// Ensure the MachinePool is owned by the Cluster it belongs to.
	mp.SetOwnerReferences(util.EnsureOwnerRef(mp.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	phases := []func(context.Context, *clusterv1.Cluster, *expv1.MachinePool) (ctrl.Result, error){
		r.reconcileBootstrap,
		r.reconcileInfrastructure,
		r.reconcileNodeRefs,
	}

	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, cluster, mp)
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

func (r *MachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, mp *expv1.MachinePool) error {
	if ok, err := r.reconcileDeleteExternal(ctx, mp); !ok || err != nil {
		// Return early and don't remove the finalizer if we got an error or
		// the external reconciliation deletion isn't ready.
		return err
	}

	if err := r.reconcileDeleteNodes(ctx, cluster, mp); err != nil {
		// Return early and don't remove the finalizer if we got an error.
		return err
	}

	controllerutil.RemoveFinalizer(mp, expv1.MachinePoolFinalizer)
	return nil
}

func (r *MachinePoolReconciler) reconcileDeleteNodes(ctx context.Context, cluster *clusterv1.Cluster, machinepool *expv1.MachinePool) error {
	if len(machinepool.Status.NodeRefs) == 0 {
		return nil
	}

	clusterClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return err
	}

	return r.deleteRetiredNodes(ctx, clusterClient, machinepool.Status.NodeRefs, machinepool.Spec.ProviderIDList)
}

// reconcileDeleteExternal tries to delete external references, returning true if it cannot find any.
func (r *MachinePoolReconciler) reconcileDeleteExternal(ctx context.Context, m *expv1.MachinePool) (bool, error) {
	objects := []*unstructured.Unstructured{}
	references := []*corev1.ObjectReference{
		m.Spec.Template.Spec.Bootstrap.ConfigRef,
		&m.Spec.Template.Spec.InfrastructureRef,
	}

	// Loop over the references and try to retrieve it with the client.
	for _, ref := range references {
		if ref == nil {
			continue
		}

		obj, err := external.Get(ctx, r.Client, ref, m.Namespace)
		if err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
			return false, errors.Wrapf(err, "failed to get %s %q for MachinePool %q in namespace %q",
				ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		if obj != nil {
			objects = append(objects, obj)
		}
	}

	// Issue a delete request for any object that has been found.
	for _, obj := range objects {
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return false, errors.Wrapf(err,
				"failed to delete %v %q for MachinePool %q in namespace %q",
				obj.GroupVersionKind(), obj.GetName(), m.Name, m.Namespace)
		}
	}

	// Return true if there are no more external objects.
	return len(objects) == 0, nil
}

func (r *MachinePoolReconciler) watchClusterNodes(ctx context.Context, cluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		log.V(5).Info("Skipping node watching setup because control plane is not initialized")
		return nil
	}

	// If there is no tracker, don't watch remote nodes
	if r.Tracker == nil {
		return nil
	}

	return r.Tracker.Watch(ctx, remote.WatchInput{
		Name:         "machinepool-watchNodes",
		Cluster:      util.ObjectKey(cluster),
		Watcher:      r.controller,
		Kind:         &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.nodeToMachinePool),
	})
}

func (r *MachinePoolReconciler) nodeToMachinePool(ctx context.Context, o client.Object) []reconcile.Request {
	node, ok := o.(*corev1.Node)
	if !ok {
		panic(fmt.Sprintf("Expected a Node but got a %T", o))
	}

	var filters []client.ListOption
	// Match by clusterName when the node has the annotation.
	if clusterName, ok := node.GetAnnotations()[clusterv1.ClusterNameAnnotation]; ok {
		filters = append(filters, client.MatchingLabels{
			clusterv1.ClusterNameLabel: clusterName,
		})
	}

	// Match by namespace when the node has the annotation.
	if namespace, ok := node.GetAnnotations()[clusterv1.ClusterNamespaceAnnotation]; ok {
		filters = append(filters, client.InNamespace(namespace))
	}

	// Match by nodeName and status.nodeRef.name.
	machinePoolList := &expv1.MachinePoolList{}
	if err := r.Client.List(
		ctx,
		machinePoolList,
		append(filters, client.MatchingFields{index.MachinePoolNodeNameField: node.Name})...); err != nil {
		return nil
	}

	// There should be exactly 1 MachinePool for the node.
	if len(machinePoolList.Items) == 1 {
		return []reconcile.Request{{NamespacedName: util.ObjectKey(&machinePoolList.Items[0])}}
	}

	// Otherwise let's match by providerID. This is useful when e.g the NodeRef has not been set yet.
	// Match by providerID
	if node.Spec.ProviderID == "" {
		return nil
	}
	machinePoolList = &expv1.MachinePoolList{}
	if err := r.Client.List(
		ctx,
		machinePoolList,
		append(filters, client.MatchingFields{index.MachinePoolProviderIDField: node.Spec.ProviderID})...); err != nil {
		return nil
	}

	// There should be exactly 1 MachinePool for the node.
	if len(machinePoolList.Items) == 1 {
		return []reconcile.Request{{NamespacedName: util.ObjectKey(&machinePoolList.Items[0])}}
	}

	return nil
}
