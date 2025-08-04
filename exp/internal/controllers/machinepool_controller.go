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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// Update permissions on /finalizers subresrouce is required on management clusters with 'OwnerReferencesPermissionEnforcement' plugin enabled.
// See: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#ownerreferencespermissionenforcement
//
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status;machinepools/finalizers,verbs=get;list;watch;create;update;patch;delete

// machinePoolKind contains the schema.GroupVersionKind for the MachinePool type.
var machinePoolKind = clusterv1.GroupVersion.WithKind("MachinePool")

const (
	// MachinePoolControllerName defines the controller used when creating clients.
	MachinePoolControllerName = "machinepool-controller"
)

// MachinePoolReconciler reconciles a MachinePool object.
type MachinePoolReconciler struct {
	Client       client.Client
	APIReader    client.Reader
	ClusterCache clustercache.ClusterCache

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	controller      controller.Controller
	ssaCache        ssa.Cache
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker

	predicateLog *logr.Logger
}

func (r *MachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil {
		return errors.New("Client, APIReader and ClusterCache must not be nil")
	}

	r.predicateLog = ptr.To(ctrl.LoggerFrom(ctx).WithValues("controller", "machinepool"))
	clusterToMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), *r.predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachinePools),
			// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
			builder.WithPredicates(
				predicates.All(mgr.GetScheme(), *r.predicateLog,
					predicates.ResourceIsChanged(mgr.GetScheme(), *r.predicateLog),
					predicates.ClusterPausedTransitions(mgr.GetScheme(), *r.predicateLog),
					predicates.ResourceHasFilterLabel(mgr.GetScheme(), *r.predicateLog, r.WatchFilterValue),
				),
			),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("machinepool", clusterToMachinePools)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("machinepool-controller")
	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: r.predicateLog,
	}
	r.ssaCache = ssa.NewCache("machinepool")

	return nil
}

func (r *MachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	mp := &clusterv1.MachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, mp); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		log.Error(err, "Error reading the object - requeue the request.")
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(mp.Namespace, mp.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, mp, clusterv1.MachinePoolFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, mp.Namespace, mp.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get Cluster for MachinePool.", "MachinePool", klog.KObj(mp), "Cluster", klog.KRef(mp.Namespace, mp.Spec.ClusterName))
		return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster %q for machinepool %q in namespace %q",
			mp.Spec.ClusterName, mp.Name, mp.Namespace)
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(mp, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, mp); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
	}

	// Handle normal reconciliation loop.
	scope := &scope{
		cluster:     cluster,
		machinePool: mp,
	}

	defer func() {
		if err := r.updateStatus(ctx, scope); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		r.reconcilePhase(mp)
		// TODO(jpang): add support for metrics.

		// Always update the readyCondition with the summary of the machinepool conditions.
		v1beta1conditions.SetSummary(mp,
			v1beta1conditions.WithConditions(
				clusterv1.BootstrapReadyV1Beta1Condition,
				clusterv1.InfrastructureReadyV1Beta1Condition,
				clusterv1.ReplicasReadyV1Beta1Condition,
			),
		)

		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{
			patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ReadyV1Beta1Condition,
				clusterv1.BootstrapReadyV1Beta1Condition,
				clusterv1.InfrastructureReadyV1Beta1Condition,
				clusterv1.ReplicasReadyV1Beta1Condition,
			}},
			patch.WithOwnedConditions{Conditions: []string{
				clusterv1.PausedCondition,
			}},
		}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, mp, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	alwaysReconcile := []machinePoolReconcileFunc{
		wrapErrMachinePoolReconcileFunc(r.reconcileSetOwnerAndLabels, "failed to set MachinePool owner and labels"),
	}

	// Handle deletion reconciliation loop.
	if !mp.DeletionTimestamp.IsZero() {
		reconcileDelete := append(
			alwaysReconcile,
			wrapErrMachinePoolReconcileFunc(r.reconcileDelete, "failed to reconcile delete"),
		)
		return doReconcile(ctx, scope, reconcileDelete)
	}

	reconcileNormal := append(alwaysReconcile,
		wrapErrMachinePoolReconcileFunc(r.reconcileBootstrap, "failed to reconcile bootstrap config"),
		wrapErrMachinePoolReconcileFunc(r.reconcileInfrastructure, "failed to reconcile infrastructure"),
		wrapErrMachinePoolReconcileFunc(r.getMachinesForMachinePool, "failed to get Machines for MachinePool"),
		wrapErrMachinePoolReconcileFunc(r.reconcileNodeRefs, "failed to reconcile nodeRefs"),
		wrapErrMachinePoolReconcileFunc(r.setMachinesUptoDate, "failed to set machines up to date"),
	)

	return doReconcile(ctx, scope, reconcileNormal)
}

func (r *MachinePoolReconciler) reconcileSetOwnerAndLabels(_ context.Context, s *scope) (ctrl.Result, error) {
	cluster := s.cluster
	mp := s.machinePool

	if mp.Labels == nil {
		mp.Labels = make(map[string]string)
	}
	mp.Labels[clusterv1.ClusterNameLabel] = mp.Spec.ClusterName

	mp.SetOwnerReferences(util.EnsureOwnerRef(mp.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	return ctrl.Result{}, nil
}

// reconcileDelete delete machinePool related resources.
func (r *MachinePoolReconciler) reconcileDelete(ctx context.Context, s *scope) (ctrl.Result, error) {
	if ok, err := r.reconcileDeleteExternal(ctx, s.machinePool); !ok || err != nil {
		// Return early and don't remove the finalizer if we got an error or
		// the external reconciliation deletion isn't ready.
		return ctrl.Result{}, fmt.Errorf("failed deleting external references: %s", err)
	}

	// check nodes delete timeout passed.
	if !r.isMachinePoolNodeDeleteTimeoutPassed(s.machinePool) {
		if err := r.reconcileDeleteNodes(ctx, s.cluster, s.machinePool); err != nil {
			// Return early and don't remove the finalizer if we got an error.
			return ctrl.Result{}, fmt.Errorf("failed deleting nodes: %w", err)
		}
	} else {
		ctrl.LoggerFrom(ctx).Info("NodeDeleteTimeout passed, skipping Nodes deletion")
	}

	controllerutil.RemoveFinalizer(s.machinePool, clusterv1.MachinePoolFinalizer)
	return ctrl.Result{}, nil
}

// reconcileDeleteNodes delete the cluster nodes.
func (r *MachinePoolReconciler) reconcileDeleteNodes(ctx context.Context, cluster *clusterv1.Cluster, machinePool *clusterv1.MachinePool) error {
	if len(machinePool.Status.NodeRefs) == 0 {
		return nil
	}

	clusterClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return err
	}

	return r.deleteRetiredNodes(ctx, clusterClient, machinePool.Status.NodeRefs, machinePool.Spec.ProviderIDList)
}

// isMachinePoolDeleteTimeoutPassed check the machinePool node delete time out.
func (r *MachinePoolReconciler) isMachinePoolNodeDeleteTimeoutPassed(machinePool *clusterv1.MachinePool) bool {
	if !machinePool.DeletionTimestamp.IsZero() && machinePool.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds != nil {
		if *machinePool.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds != 0 {
			deleteTimePlusDuration := machinePool.DeletionTimestamp.Add(time.Duration(*machinePool.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds) * time.Second)
			return deleteTimePlusDuration.Before(time.Now())
		}
	}
	return false
}

// reconcileDeleteExternal tries to delete external references, returning true if it cannot find any.
func (r *MachinePoolReconciler) reconcileDeleteExternal(ctx context.Context, machinePool *clusterv1.MachinePool) (bool, error) {
	objects := []*unstructured.Unstructured{}
	references := []clusterv1.ContractVersionedObjectReference{
		machinePool.Spec.Template.Spec.Bootstrap.ConfigRef,
		machinePool.Spec.Template.Spec.InfrastructureRef,
	}

	// Loop over the references and try to retrieve it with the client.
	for _, ref := range references {
		if !ref.IsDefined() {
			continue
		}

		obj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, ref, machinePool.Namespace)
		if err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
			return false, errors.Wrapf(err, "failed to get %s %s for MachinePool %s",
				ref.Kind, klog.KRef(machinePool.Namespace, ref.Name), klog.KObj(machinePool))
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
				obj.GroupVersionKind(), obj.GetName(), machinePool.Name, machinePool.Namespace)
		}
	}

	// Return true if there are no more external objects.
	return len(objects) == 0, nil
}

func (r *MachinePoolReconciler) watchClusterNodes(ctx context.Context, cluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	if !conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
		log.V(5).Info("Skipping node watching setup because control plane is not initialized")
		return nil
	}

	return r.ClusterCache.Watch(ctx, util.ObjectKey(cluster), clustercache.NewWatcher(clustercache.WatcherOptions{
		Name:         "machinepool-watchNodes",
		Watcher:      r.controller,
		Kind:         &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.nodeToMachinePool),
		Predicates:   []predicate.TypedPredicate[client.Object]{predicates.TypedResourceIsChanged[client.Object](r.Client.Scheme(), *r.predicateLog)},
	}))
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
	machinePoolList := &clusterv1.MachinePoolList{}
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
	machinePoolList = &clusterv1.MachinePoolList{}
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

func (r *MachinePoolReconciler) getMachinesForMachinePool(ctx context.Context, s *scope) (ctrl.Result, error) {
	infraMachineSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			clusterv1.MachinePoolNameLabel: format.MustFormatValue(s.machinePool.Name),
			clusterv1.ClusterNameLabel:     s.machinePool.Spec.ClusterName,
		},
	}

	allMachines := &clusterv1.MachineList{}
	if err := r.Client.List(ctx,
		allMachines,
		client.InNamespace(s.machinePool.Namespace),
		client.MatchingLabels(infraMachineSelector.MatchLabels)); err != nil {
		return ctrl.Result{}, err
	}

	filteredMachines := make([]*clusterv1.Machine, 0, len(allMachines.Items))
	for idx := range allMachines.Items {
		machine := &allMachines.Items[idx]
		if shouldExcludeMachine(s.machinePool, machine) {
			continue
		}
		filteredMachines = append(filteredMachines, machine)
	}

	s.machines = filteredMachines

	return ctrl.Result{}, nil
}

func (r *MachinePoolReconciler) setMachinesUptoDate(ctx context.Context, s *scope) (ctrl.Result, error) {
	var errs []error
	for _, machine := range s.machines {
		patchHelper, err := patch.NewHelper(machine, r.Client)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		upToDateCondition := &metav1.Condition{
			Type: clusterv1.MachineUpToDateCondition,
		}

		if machine.DeletionTimestamp.IsZero() {
			upToDateCondition.Status = metav1.ConditionTrue
			upToDateCondition.Reason = clusterv1.MachineUpToDateReason
		} else {
			upToDateCondition.Status = metav1.ConditionFalse
			upToDateCondition.Reason = clusterv1.MachineNotUpToDateReason
			upToDateCondition.Message = "Machine is being deleted"
		}
		conditions.Set(machine, *upToDateCondition)

		if err := patchHelper.Patch(ctx, machine, patch.WithOwnedConditions{Conditions: []string{
			clusterv1.MachineUpToDateCondition,
		}}); err != nil {
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	return ctrl.Result{}, nil
}

type machinePoolReconcileFunc func(ctx context.Context, s *scope) (ctrl.Result, error)

func wrapErrMachinePoolReconcileFunc(f machinePoolReconcileFunc, msg string) machinePoolReconcileFunc {
	return func(ctx context.Context, s *scope) (ctrl.Result, error) {
		res, err := f(ctx, s)
		return res, errors.Wrap(err, msg)
	}
}

func doReconcile(ctx context.Context, s *scope, phases []machinePoolReconcileFunc) (ctrl.Result, kerrors.Aggregate) {
	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, s)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}

	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	return res, nil
}

func shouldExcludeMachine(machinePool *clusterv1.MachinePool, machine *clusterv1.Machine) bool {
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machinePool) {
		return true
	}

	return false
}
