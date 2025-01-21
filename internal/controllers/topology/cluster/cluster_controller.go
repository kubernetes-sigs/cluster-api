/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	externalfake "sigs.k8s.io/cluster-api/controllers/external/fake"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/desiredstate"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinehealthchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;create;delete

// Reconciler reconciles a managed topology for a Cluster object.
type Reconciler struct {
	Client       client.Client
	ClusterCache clustercache.ClusterCache
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader client.Reader

	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	externalTracker external.ObjectTracker
	recorder        record.EventRecorder

	// desiredStateGenerator is used to generate the desired state.
	desiredStateGenerator desiredstate.Generator

	patchHelperFactory structuredmerge.PatchHelperFactoryFunc
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil {
		return errors.New("Client, APIReader and ClusterCache must not be nil")
	}

	if feature.Gates.Enabled(feature.RuntimeSDK) && r.RuntimeClient == nil {
		return errors.New("RuntimeClient must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "topology/cluster")
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}, builder.WithPredicates(
			// Only reconcile Cluster with topology and with changes relevant for this controller.
			predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ClusterHasTopology(mgr.GetScheme(), predicateLog),
				clusterChangeIsRelevant(mgr.GetScheme(), predicateLog),
			),
		)).
		Named("topology/cluster").
		WatchesRawSource(r.ClusterCache.GetClusterSource("topology/cluster", func(_ context.Context, o client.Object) []ctrl.Request {
			return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
		})).
		Watches(
			&clusterv1.ClusterClass{},
			handler.EnqueueRequestsFromMapFunc(r.clusterClassToCluster),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.MachineDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.machineDeploymentToCluster),
			// Only trigger Cluster reconciliation if the MachineDeployment is topology owned, the resource is changed, and the change is relevant.
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ResourceIsTopologyOwned(mgr.GetScheme(), predicateLog),
				machineDeploymentChangeIsRelevant(mgr.GetScheme(), predicateLog),
			)),
		).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(r.machinePoolToCluster),
			// Only trigger Cluster reconciliation if the MachinePool is topology owned, the resource is changed.
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ResourceIsTopologyOwned(mgr.GetScheme(), predicateLog),
			)),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &predicateLog,
	}
	r.desiredStateGenerator = desiredstate.NewGenerator(r.Client, r.ClusterCache, r.RuntimeClient)
	r.recorder = mgr.GetEventRecorderFor("topology/cluster-controller")
	if r.patchHelperFactory == nil {
		r.patchHelperFactory = serverSideApplyPatchHelperFactory(r.Client, ssa.NewCache("topology/cluster"))
	}
	return nil
}

func clusterChangeIsRelevant(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	dropNotRelevant := func(cluster *clusterv1.Cluster) *clusterv1.Cluster {
		c := cluster.DeepCopy()
		// Drop metadata fields which are impacted by not relevant changes.
		c.ObjectMeta.ManagedFields = nil
		c.ObjectMeta.ResourceVersion = ""

		// Drop changes on v1beta2 conditions; when v1beta2 conditions will be moved top level, we will review this
		// selectively drop changes not relevant for this controller.
		c.Status.V1Beta2 = nil
		return c
	}

	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "ClusterChangeIsRelevant", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
				log.V(6).Info("Cluster resync event, allowing further processing")
				return true
			}

			oldObj, ok := e.ObjectOld.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			oldObj = dropNotRelevant(oldObj)

			newObj := e.ObjectNew.(*clusterv1.Cluster)
			if !ok {
				log.V(4).Info("Expected Cluster", "type", fmt.Sprintf("%T", e.ObjectNew))
				return false
			}
			newObj = dropNotRelevant(newObj)

			if reflect.DeepEqual(oldObj, newObj) {
				log.V(6).Info("Cluster does not have relevant changes, blocking further processing")
				return false
			}
			log.V(6).Info("Cluster has relevant changes, allowing further processing")
			return true
		},
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
	}
}

func machineDeploymentChangeIsRelevant(scheme *runtime.Scheme, logger logr.Logger) predicate.Funcs {
	dropNotRelevant := func(machineDeployment *clusterv1.MachineDeployment) *clusterv1.MachineDeployment {
		md := machineDeployment.DeepCopy()
		// Drop metadata fields which are impacted by not relevant changes.
		md.ObjectMeta.ManagedFields = nil
		md.ObjectMeta.ResourceVersion = ""

		// Drop changes on v1beta2 conditions; when v1beta2 conditions will be moved top level, we will review this
		// selectively drop changes not relevant for this controller.
		md.Status.V1Beta2 = nil
		return md
	}

	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log := logger.WithValues("predicate", "MachineDeploymentChangeIsRelevant", "eventType", "update")
			if gvk, err := apiutil.GVKForObject(e.ObjectOld, scheme); err == nil {
				log = log.WithValues(gvk.Kind, klog.KObj(e.ObjectOld))
			}

			oldObj, ok := e.ObjectOld.(*clusterv1.MachineDeployment)
			if !ok {
				log.V(4).Info("Expected MachineDeployment", "type", fmt.Sprintf("%T", e.ObjectOld))
				return false
			}
			oldObj = dropNotRelevant(oldObj)

			newObj := e.ObjectNew.(*clusterv1.MachineDeployment)
			if !ok {
				log.V(4).Info("Expected MachineDeployment", "type", fmt.Sprintf("%T", e.ObjectNew))
				return false
			}
			newObj = dropNotRelevant(newObj)

			if reflect.DeepEqual(oldObj, newObj) {
				log.V(6).Info("MachineDeployment does not have relevant changes, blocking further processing")
				return false
			}
			log.V(6).Info("MachineDeployment has relevant changes, allowing further processing")
			return true
		},
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
	}
}

// SetupForDryRun prepares the Reconciler for a dry run execution.
func (r *Reconciler) SetupForDryRun(recorder record.EventRecorder) {
	r.desiredStateGenerator = desiredstate.NewGenerator(r.Client, r.ClusterCache, r.RuntimeClient)
	r.recorder = recorder
	r.externalTracker = external.ObjectTracker{
		Controller:      externalfake.Controller{},
		Cache:           &informertest.FakeInformers{},
		Scheme:          r.Client.Scheme(),
		PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
	}
	r.patchHelperFactory = dryRunPatchHelperFactory(r.Client)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	cluster.APIVersion = clusterv1.GroupVersion.String()
	cluster.Kind = "Cluster"

	// Return early, if the Cluster does not use a managed topology.
	// NOTE: We're already filtering events, but this is a safeguard for cases like e.g. when
	// there are MachineDeployments which have the topology owned label, but the corresponding
	// cluster is not topology owned.
	if cluster.Spec.Topology == nil {
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(cluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create a scope initialized with only the cluster; during reconcile
	// additional information will be added about the Cluster blueprint, current state and desired state.
	s := scope.New(cluster)

	defer func() {
		if err := r.reconcileConditions(s, cluster, reterr); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrap(err, "failed to reconcile cluster topology conditions")})
			return
		}
		options := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.TopologyReconciledCondition,
			}},
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ClusterTopologyReconciledV1Beta2Condition,
			}},
		}
		if err := patchHelper.Patch(ctx, cluster, options...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
			return
		}
	}()

	// Return early if the Cluster is paused.
	if cluster.Spec.Paused || annotations.HasPaused(cluster) {
		return ctrl.Result{}, nil
	}

	// In case the object is deleted, the managed topology stops to reconcile;
	// (the other controllers will take care of deletion).
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, s)
}

// reconcile handles cluster reconciliation.
func (r *Reconciler) reconcile(ctx context.Context, s *scope.Scope) (ctrl.Result, error) {
	var err error

	// Get ClusterClass.
	clusterClass := &clusterv1.ClusterClass{}
	key := s.Current.Cluster.GetClassKey()
	if err := r.Client.Get(ctx, key, clusterClass); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve ClusterClass %s", key)
	}

	s.Blueprint.ClusterClass = clusterClass
	// If the ClusterClass `metadata.Generation` doesn't match the `status.ObservedGeneration` return as the ClusterClass
	// is not up to date.
	// Note: This doesn't require requeue as a change to ClusterClass observedGeneration will cause an additional reconcile
	// in the Cluster.
	if !conditions.Has(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) ||
		conditions.IsFalse(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition) {
		return ctrl.Result{}, errors.Errorf("ClusterClass is not successfully reconciled: status of %s condition on ClusterClass must be \"True\"", clusterv1.ClusterClassVariablesReconciledCondition)
	}
	if clusterClass.GetGeneration() != clusterClass.Status.ObservedGeneration {
		return ctrl.Result{}, errors.Errorf("ClusterClass is not successfully reconciled: ClusterClass.status.observedGeneration must be %d, but is %d", clusterClass.GetGeneration(), clusterClass.Status.ObservedGeneration)
	}

	// Default and Validate the Cluster variables based on information from the ClusterClass.
	// This step is needed as if the ClusterClass does not exist at Cluster creation some fields may not be defaulted or
	// validated in the webhook.
	if errs := webhooks.DefaultAndValidateVariables(ctx, s.Current.Cluster, nil, clusterClass); len(errs) > 0 {
		return ctrl.Result{}, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), s.Current.Cluster.Name, errs)
	}

	// Gets the blueprint with the ClusterClass and the referenced templates
	// and store it in the request scope.
	s.Blueprint, err = r.getBlueprint(ctx, s.Current.Cluster, s.Blueprint.ClusterClass)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reading the ClusterClass")
	}

	// Gets the current state of the Cluster and store it in the request scope.
	s.Current, err = r.getCurrentState(ctx, s)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reading current state of the Cluster topology")
	}

	// The cluster topology is yet to be created. Call the BeforeClusterCreate hook before proceeding.
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		res, err := r.callBeforeClusterCreateHook(ctx, s)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !res.IsZero() {
			return res, nil
		}
	}

	// Setup watches for InfrastructureCluster and ControlPlane CRs when they exist.
	if err := r.setupDynamicWatches(ctx, s); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error creating dynamic watch")
	}

	// Computes the desired state of the Cluster and store it in the request scope.
	s.Desired, err = r.desiredStateGenerator.Generate(ctx, s)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error computing the desired state of the Cluster topology")
	}

	// Reconciles current and desired state of the Cluster
	if err := r.reconcileState(ctx, s); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reconciling the Cluster topology")
	}

	// requeueAfter will not be 0 if any of the runtime hooks returns a blocking response.
	requeueAfter := s.HookResponseTracker.AggregateRetryAfter()
	if requeueAfter != 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// setupDynamicWatches create watches for InfrastructureCluster and ControlPlane CRs when they exist.
func (r *Reconciler) setupDynamicWatches(ctx context.Context, s *scope.Scope) error {
	scheme := r.Client.Scheme()
	if s.Current.InfrastructureCluster != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.InfrastructureCluster,
			handler.EnqueueRequestForOwner(scheme, r.Client.RESTMapper(), &clusterv1.Cluster{}),
			// Only trigger Cluster reconciliation if the InfrastructureCluster is topology owned.
			predicates.All(scheme, *r.externalTracker.PredicateLogger,
				predicates.ResourceIsChanged(scheme, *r.externalTracker.PredicateLogger),
				predicates.ResourceIsTopologyOwned(scheme, *r.externalTracker.PredicateLogger),
			)); err != nil {
			return errors.Wrap(err, "error watching Infrastructure CR")
		}
	}
	if s.Current.ControlPlane.Object != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.ControlPlane.Object,
			handler.EnqueueRequestForOwner(scheme, r.Client.RESTMapper(), &clusterv1.Cluster{}),
			// Only trigger Cluster reconciliation if the ControlPlane is topology owned.
			predicates.All(scheme, *r.externalTracker.PredicateLogger,
				predicates.ResourceIsChanged(scheme, *r.externalTracker.PredicateLogger),
				predicates.ResourceIsTopologyOwned(scheme, *r.externalTracker.PredicateLogger),
			)); err != nil {
			return errors.Wrap(err, "error watching ControlPlane CR")
		}
	}
	return nil
}

func (r *Reconciler) callBeforeClusterCreateHook(ctx context.Context, s *scope.Scope) (reconcile.Result, error) {
	// If the cluster objects (InfraCluster, ControlPlane, etc) are not yet created we are in the creation phase.
	// Call the BeforeClusterCreate hook before proceeding.
	log := ctrl.LoggerFrom(ctx)
	if s.Current.Cluster.Spec.InfrastructureRef == nil && s.Current.Cluster.Spec.ControlPlaneRef == nil {
		hookRequest := &runtimehooksv1.BeforeClusterCreateRequest{
			Cluster: *s.Current.Cluster,
		}
		hookResponse := &runtimehooksv1.BeforeClusterCreateResponse{}
		if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.BeforeClusterCreate, s.Current.Cluster, hookRequest, hookResponse); err != nil {
			return ctrl.Result{}, err
		}
		s.HookResponseTracker.Add(runtimehooksv1.BeforeClusterCreate, hookResponse)
		if hookResponse.RetryAfterSeconds != 0 {
			log.Info(fmt.Sprintf("Creation of Cluster topology is blocked by %s hook", runtimecatalog.HookName(runtimehooksv1.BeforeClusterCreate)))
			return ctrl.Result{RequeueAfter: time.Duration(hookResponse.RetryAfterSeconds) * time.Second}, nil
		}
	}
	return ctrl.Result{}, nil
}

// clusterClassToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when its own ClusterClass gets updated.
func (r *Reconciler) clusterClassToCluster(ctx context.Context, o client.Object) []ctrl.Request {
	clusterClass, ok := o.(*clusterv1.ClusterClass)
	if !ok {
		panic(fmt.Sprintf("Expected a ClusterClass but got a %T", o))
	}

	clusterList := &clusterv1.ClusterList{}
	if err := r.Client.List(
		ctx,
		clusterList,
		client.MatchingFields{
			index.ClusterClassRefPath: index.ClusterClassRef(clusterClass),
		},
	); err != nil {
		return nil
	}

	// There can be more than one cluster using the same cluster class.
	// create a request for each of the clusters.
	requests := []ctrl.Request{}
	for i := range clusterList.Items {
		requests = append(requests, ctrl.Request{NamespacedName: util.ObjectKey(&clusterList.Items[i])})
	}
	return requests
}

// machineDeploymentToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when one of its own MachineDeployments gets updated.
func (r *Reconciler) machineDeploymentToCluster(_ context.Context, o client.Object) []ctrl.Request {
	md, ok := o.(*clusterv1.MachineDeployment)
	if !ok {
		panic(fmt.Sprintf("Expected a MachineDeployment but got a %T", o))
	}
	if md.Spec.ClusterName == "" {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: md.Namespace,
			Name:      md.Spec.ClusterName,
		},
	}}
}

// machinePoolToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when one of its own MachinePools gets updated.
func (r *Reconciler) machinePoolToCluster(_ context.Context, o client.Object) []ctrl.Request {
	mp, ok := o.(*expv1.MachinePool)
	if !ok {
		panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
	}
	if mp.Spec.ClusterName == "" {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: mp.Namespace,
			Name:      mp.Spec.ClusterName,
		},
	}}
}

func (r *Reconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	// Call the BeforeClusterDelete hook if the 'ok-to-delete' annotation is not set
	// and add the annotation to the cluster after receiving a successful non-blocking response.
	log := ctrl.LoggerFrom(ctx)
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		if !hooks.IsOkToDelete(cluster) {
			hookRequest := &runtimehooksv1.BeforeClusterDeleteRequest{
				Cluster: *cluster,
			}
			hookResponse := &runtimehooksv1.BeforeClusterDeleteResponse{}
			if err := r.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.BeforeClusterDelete, cluster, hookRequest, hookResponse); err != nil {
				return ctrl.Result{}, err
			}
			if hookResponse.RetryAfterSeconds != 0 {
				log.Info(fmt.Sprintf("Cluster deletion is blocked by %q hook", runtimecatalog.HookName(runtimehooksv1.BeforeClusterDelete)))
				return ctrl.Result{RequeueAfter: time.Duration(hookResponse.RetryAfterSeconds) * time.Second}, nil
			}
			// The BeforeClusterDelete hook returned a non-blocking response. Now the cluster is ready to be deleted.
			// Lets mark the cluster as `ok-to-delete`
			if err := hooks.MarkAsOkToDelete(ctx, r.Client, cluster); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

// serverSideApplyPatchHelperFactory makes use of managed fields provided by server side apply and is used by the controller.
func serverSideApplyPatchHelperFactory(c client.Client, ssaCache ssa.Cache) structuredmerge.PatchHelperFactoryFunc {
	return func(ctx context.Context, original, modified client.Object, opts ...structuredmerge.HelperOption) (structuredmerge.PatchHelper, error) {
		return structuredmerge.NewServerSidePatchHelper(ctx, original, modified, c, ssaCache, opts...)
	}
}

// dryRunPatchHelperFactory makes use of a two-ways patch and is used in situations where we cannot rely on managed fields.
func dryRunPatchHelperFactory(c client.Client) structuredmerge.PatchHelperFactoryFunc {
	return func(_ context.Context, original, modified client.Object, opts ...structuredmerge.HelperOption) (structuredmerge.PatchHelper, error) {
		return structuredmerge.NewTwoWaysPatchHelper(original, modified, c, opts...)
	}
}
