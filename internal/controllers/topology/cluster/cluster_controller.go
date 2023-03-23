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
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/external"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/hooks"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinehealthchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;create;delete

// Reconciler reconciles a managed topology for a Cluster object.
type Reconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader client.Reader

	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// UnstructuredCachingClient provides a client that forces caching of unstructured objects,
	// thus allowing to optimize reads for templates or provider specific objects in a managed topology.
	UnstructuredCachingClient client.Client

	externalTracker external.ObjectTracker
	recorder        record.EventRecorder

	// patchEngine is used to apply patches during computeDesiredState.
	patchEngine patches.Engine

	patchHelperFactory structuredmerge.PatchHelperFactoryFunc
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}, builder.WithPredicates(
			// Only reconcile Cluster with topology.
			predicates.ClusterHasTopology(ctrl.LoggerFrom(ctx)),
		)).
		Named("topology/cluster").
		Watches(
			&source.Kind{Type: &clusterv1.ClusterClass{}},
			handler.EnqueueRequestsFromMapFunc(r.clusterClassToCluster),
		).
		Watches(
			&source.Kind{Type: &clusterv1.MachineDeployment{}},
			handler.EnqueueRequestsFromMapFunc(r.machineDeploymentToCluster),
			// Only trigger Cluster reconciliation if the MachineDeployment is topology owned.
			builder.WithPredicates(predicates.ResourceIsTopologyOwned(ctrl.LoggerFrom(ctx))),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.externalTracker = external.ObjectTracker{
		Controller: c,
	}
	r.patchEngine = patches.NewEngine(r.RuntimeClient)
	r.recorder = mgr.GetEventRecorderFor("topology/cluster")
	if r.patchHelperFactory == nil {
		r.patchHelperFactory = serverSideApplyPatchHelperFactory(r.Client, ssa.NewCache())
	}
	return nil
}

// SetupForDryRun prepares the Reconciler for a dry run execution.
func (r *Reconciler) SetupForDryRun(recorder record.EventRecorder) {
	r.patchEngine = patches.NewEngine(r.RuntimeClient)
	r.recorder = recorder
	r.patchHelperFactory = dryRunPatchHelperFactory(r.Client)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	// Use the live client here so that we do not reconcile a stale cluster object.
	// Example: If 2 reconcile loops are triggered in quick succession (one from the cluster and the other from the clusterclass)
	// the first reconcile loop could update the cluster object (set the infrastructure cluster ref and control plane ref). If we
	// do not use the live client the second reconcile loop could potentially pick up the stale cluster object from the cache.
	if err := r.APIReader.Get(ctx, req.NamespacedName, cluster); err != nil {
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

	// Return early if the Cluster is paused.
	// TODO: What should we do if the cluster class is paused?
	if annotations.IsPaused(cluster, cluster) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// In case the object is deleted, the managed topology stops to reconcile;
	// (the other controllers will take care of deletion).
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
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
			patch.WithForceOverwriteConditions{},
		}
		if err := patchHelper.Patch(ctx, cluster, options...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrap(err, "failed to patch cluster")})
			return
		}
	}()

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, s)
}

// reconcile handles cluster reconciliation.
func (r *Reconciler) reconcile(ctx context.Context, s *scope.Scope) (ctrl.Result, error) {
	var err error

	// Get ClusterClass.
	clusterClass := &clusterv1.ClusterClass{}
	key := client.ObjectKey{Name: s.Current.Cluster.Spec.Topology.Class, Namespace: s.Current.Cluster.Namespace}
	if err := r.Client.Get(ctx, key, clusterClass); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve ClusterClass %s", s.Current.Cluster.Spec.Topology.Class)
	}

	s.Blueprint.ClusterClass = clusterClass
	// If the ClusterClass `metadata.Generation` doesn't match the `status.ObservedGeneration` return as the ClusterClass
	// is not up to date.
	// Note: This doesn't require requeue as a change to ClusterClass observedGeneration will cause an additional reconcile
	// in the Cluster.
	if clusterClass.GetGeneration() != clusterClass.Status.ObservedGeneration {
		return ctrl.Result{}, nil
	}

	// Default and Validate the Cluster variables based on information from the ClusterClass.
	// This step is needed as if the ClusterClass does not exist at Cluster creation some fields may not be defaulted or
	// validated in the webhook.
	if errs := webhooks.DefaultAndValidateVariables(s.Current.Cluster, clusterClass); len(errs) > 0 {
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
	s.Desired, err = r.computeDesiredState(ctx, s)
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
	if s.Current.InfrastructureCluster != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.InfrastructureCluster,
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}},
			// Only trigger Cluster reconciliation if the InfrastructureCluster is topology owned.
			predicates.ResourceIsTopologyOwned(ctrl.LoggerFrom(ctx))); err != nil {
			return errors.Wrap(err, "error watching Infrastructure CR")
		}
	}
	if s.Current.ControlPlane.Object != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.ControlPlane.Object,
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}},
			// Only trigger Cluster reconciliation if the ControlPlane is topology owned.
			predicates.ResourceIsTopologyOwned(ctrl.LoggerFrom(ctx))); err != nil {
			return errors.Wrap(err, "error watching ControlPlane CR")
		}
	}
	return nil
}

func (r *Reconciler) callBeforeClusterCreateHook(ctx context.Context, s *scope.Scope) (reconcile.Result, error) {
	// If the cluster objects (InfraCluster, ControlPlane, etc) are not yet created we are in the creation phase.
	// Call the BeforeClusterCreate hook before proceeding.
	log := tlog.LoggerFrom(ctx)
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
			log.Infof("Creation of Cluster topology is blocked by %s hook", runtimecatalog.HookName(runtimehooksv1.BeforeClusterCreate))
			return ctrl.Result{RequeueAfter: time.Duration(hookResponse.RetryAfterSeconds) * time.Second}, nil
		}
	}
	return ctrl.Result{}, nil
}

// clusterClassToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when its own ClusterClass gets updated.
func (r *Reconciler) clusterClassToCluster(o client.Object) []ctrl.Request {
	clusterClass, ok := o.(*clusterv1.ClusterClass)
	if !ok {
		panic(fmt.Sprintf("Expected a ClusterClass but got a %T", o))
	}

	clusterList := &clusterv1.ClusterList{}
	if err := r.Client.List(
		context.TODO(),
		clusterList,
		client.MatchingFields{index.ClusterClassNameField: clusterClass.Name},
		client.InNamespace(clusterClass.Namespace),
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
func (r *Reconciler) machineDeploymentToCluster(o client.Object) []ctrl.Request {
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

func (r *Reconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	// Call the BeforeClusterDelete hook if the 'ok-to-delete' annotation is not set
	// and add the annotation to the cluster after receiving a successful non-blocking response.
	log := tlog.LoggerFrom(ctx)
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
				log.Infof("Cluster deletion is blocked by %q hook", runtimecatalog.HookName(runtimehooksv1.BeforeClusterDelete))
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
	return func(ctx context.Context, original, modified client.Object, opts ...structuredmerge.HelperOption) (structuredmerge.PatchHelper, error) {
		return structuredmerge.NewTwoWaysPatchHelper(original, modified, c, opts...)
	}
}
