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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/finalizers"
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

// scope holds the different objects that are read and used during the reconcile.
type scope struct {
	// cluster is the Cluster object the Machine belongs to.
	// It is set at the beginning of the reconcile function.
	cluster *clusterv1.Cluster

	// machinePool is the MachinePool object. It is set at the beginning
	// of the reconcile function.
	machinePool *expv1.MachinePool

	// nodeRefMapResult is a map of providerIDs to Nodes that are associated with the Cluster.
	// It is set after reconcileInfrastructure is called.
	nodeRefMap map[string]*corev1.Node
}

func (r *MachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil {
		return errors.New("Client, APIReader and ClusterCache must not be nil")
	}

	r.predicateLog = ptr.To(ctrl.LoggerFrom(ctx).WithValues("controller", "machinepool"))
	clusterToMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &expv1.MachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&expv1.MachinePool{}).
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

	log = log.WithValues("Cluster", klog.KRef(mp.Namespace, mp.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, mp, expv1.MachinePoolFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, mp.ObjectMeta.Namespace, mp.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get Cluster for MachinePool.", "MachinePool", klog.KObj(mp), "Cluster", klog.KRef(mp.ObjectMeta.Namespace, mp.Spec.ClusterName))
		return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster %q for machinepool %q in namespace %q",
			mp.Spec.ClusterName, mp.Name, mp.Namespace)
	}

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, mp); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
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
		// Requeue if the reconcile failed because connection to workload cluster was down.
		if errors.Is(err, clustercache.ErrClusterNotConnected) {
			log.V(5).Info("Requeuing because connection to the workload cluster is down")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		return ctrl.Result{}, err
	}

	// Handle normal reconciliation loop.
	scope := &scope{
		cluster:     cluster,
		machinePool: mp,
	}
	res, err := r.reconcile(ctx, scope)
	// Requeue if the reconcile failed because connection to workload cluster was down.
	if errors.Is(err, clustercache.ErrClusterNotConnected) {
		log.V(5).Info("Requeuing because connection to the workload cluster is down")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return res, err
}

func (r *MachinePoolReconciler) reconcile(ctx context.Context, s *scope) (ctrl.Result, error) {
	// Ensure the MachinePool is owned by the Cluster it belongs to.
	cluster := s.cluster
	mp := s.machinePool
	mp.SetOwnerReferences(util.EnsureOwnerRef(mp.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	phases := []func(context.Context, *scope) (ctrl.Result, error){
		r.reconcileBootstrap,
		r.reconcileInfrastructure,
		r.reconcileNodeRefs,
	}

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
	return res, kerrors.NewAggregate(errs)
}

// reconcileDelete delete machinePool related resources.
func (r *MachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool) error {
	if ok, err := r.reconcileDeleteExternal(ctx, machinePool); !ok || err != nil {
		// Return early and don't remove the finalizer if we got an error or
		// the external reconciliation deletion isn't ready.
		return err
	}

	// check nodes delete timeout passed.
	if !r.isMachinePoolNodeDeleteTimeoutPassed(machinePool) {
		if err := r.reconcileDeleteNodes(ctx, cluster, machinePool); err != nil {
			// Return early and don't remove the finalizer if we got an error.
			return err
		}
	} else {
		ctrl.LoggerFrom(ctx).Info("NodeDeleteTimeout passed, skipping Nodes deletion")
	}

	controllerutil.RemoveFinalizer(machinePool, expv1.MachinePoolFinalizer)
	return nil
}

// reconcileDeleteNodes delete the cluster nodes.
func (r *MachinePoolReconciler) reconcileDeleteNodes(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool) error {
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
func (r *MachinePoolReconciler) isMachinePoolNodeDeleteTimeoutPassed(machinePool *expv1.MachinePool) bool {
	if !machinePool.DeletionTimestamp.IsZero() && machinePool.Spec.Template.Spec.NodeDeletionTimeout != nil {
		if machinePool.Spec.Template.Spec.NodeDeletionTimeout.Duration.Nanoseconds() != 0 {
			deleteTimePlusDuration := machinePool.DeletionTimestamp.Add(machinePool.Spec.Template.Spec.NodeDeletionTimeout.Duration)
			return deleteTimePlusDuration.Before(time.Now())
		}
	}
	return false
}

// reconcileDeleteExternal tries to delete external references, returning true if it cannot find any.
func (r *MachinePoolReconciler) reconcileDeleteExternal(ctx context.Context, machinePool *expv1.MachinePool) (bool, error) {
	objects := []*unstructured.Unstructured{}
	references := []*corev1.ObjectReference{
		machinePool.Spec.Template.Spec.Bootstrap.ConfigRef,
		&machinePool.Spec.Template.Spec.InfrastructureRef,
	}

	// Loop over the references and try to retrieve it with the client.
	for _, ref := range references {
		if ref == nil {
			continue
		}

		obj, err := external.Get(ctx, r.Client, ref)
		if err != nil && !apierrors.IsNotFound(errors.Cause(err)) {
			return false, errors.Wrapf(err, "failed to get %s %q for MachinePool %q in namespace %q",
				ref.GroupVersionKind(), ref.Name, machinePool.Name, ref.Namespace)
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

	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
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
