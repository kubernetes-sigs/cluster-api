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

package machinedeployment

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/feature"
	clientutil "sigs.k8s.io/cluster-api/internal/util/client"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var (
	// machineDeploymentKind contains the schema.GroupVersionKind for the MachineDeployment type.
	machineDeploymentKind = clusterv1.GroupVersion.WithKind("MachineDeployment")
)

// machineDeploymentManagerName is the manager name used for Server-Side-Apply (SSA) operations
// in the MachineDeployment controller.
const machineDeploymentManagerName = "capi-machinedeployment"

// Update permissions on /finalizers subresrouce is required on management clusters with 'OwnerReferencesPermissionEnforcement' plugin enabled.
// See: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#ownerreferencespermissionenforcement
//
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/status;machinedeployments/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconciler reconciles a MachineDeployment object.
type Reconciler struct {
	Client        client.Client
	APIReader     client.Reader
	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	recorder record.EventRecorder
	ssaCache ssa.Cache

	canUpdateMachineSetCache cache.Cache[CanUpdateMachineSetCacheEntry]
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil {
		return errors.New("Client and APIReader must not be nil")
	}
	if feature.Gates.Enabled(feature.InPlaceUpdates) && r.RuntimeClient == nil {
		return errors.New("RuntimeClient must not be nil when InPlaceUpdates feature gate is enabled")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "machinedeployment")
	clusterToMachineDeployments, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineDeploymentList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&clusterv1.MachineDeployment{}).
		Owns(&clusterv1.MachineSet{}).
		// Watches enqueues MachineDeployment for corresponding MachineSet resources, if no managed controller reference (owner) exists.
		Watches(
			&clusterv1.MachineSet{},
			handler.EnqueueRequestsFromMapFunc(r.MachineSetToDeployments),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachineDeployments),
			predicates.ClusterPausedTransitions(mgr.GetScheme(), predicateLog),
			// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.canUpdateMachineSetCache = cache.New[CanUpdateMachineSetCacheEntry](cache.HookCacheDefaultTTL)
	r.recorder = mgr.GetEventRecorderFor("machinedeployment-controller")
	r.ssaCache = ssa.NewCache("machinedeployment")
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (retres ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the MachineDeployment instance.
	deployment := &clusterv1.MachineDeployment{}
	if err := r.Client.Get(ctx, req.NamespacedName, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(deployment.Namespace, deployment.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, deployment, clusterv1.MachineDeploymentFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, deployment.Namespace, deployment.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(deployment, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, deployment); err != nil || isPaused || requeue {
		return ctrl.Result{}, err
	}

	s := &scope{
		machineDeployment: deployment,
		cluster:           cluster,
	}

	// Get machines.
	selectorMap, err := metav1.LabelSelectorAsMap(&s.machineDeployment.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to convert label selector to a map")
	}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(s.machineDeployment.Namespace), client.MatchingLabels(selectorMap)); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to list Machines")
	}
	s.machines = collections.FromMachineList(machineList)

	defer func() {
		if err := r.updateStatus(ctx, s); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchMachineDeployment(ctx, patchHelper, deployment, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Handle deletion reconciliation loop.
	if !deployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, s)
	}

	return ctrl.Result{}, r.reconcile(ctx, s)
}

type scope struct {
	machineDeployment                            *clusterv1.MachineDeployment
	cluster                                      *clusterv1.Cluster
	machineSets                                  []*clusterv1.MachineSet
	machines                                     collections.Machines
	bootstrapTemplateNotFound                    bool
	bootstrapTemplateExists                      bool
	infrastructureTemplateNotFound               bool
	infrastructureTemplateExists                 bool
	getAndAdoptMachineSetsForDeploymentSucceeded bool
}

func patchMachineDeployment(ctx context.Context, patchHelper *patch.Helper, md *clusterv1.MachineDeployment, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	v1beta1conditions.SetSummary(md,
		v1beta1conditions.WithConditions(
			clusterv1.MachineDeploymentAvailableV1Beta1Condition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	options = append(options,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			clusterv1.MachineDeploymentAvailableV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			clusterv1.MachineDeploymentAvailableCondition,
			clusterv1.MachineDeploymentMachinesReadyCondition,
			clusterv1.MachineDeploymentMachinesUpToDateCondition,
			clusterv1.MachineDeploymentRollingOutCondition,
			clusterv1.MachineDeploymentScalingDownCondition,
			clusterv1.MachineDeploymentScalingUpCondition,
			clusterv1.MachineDeploymentRemediatingCondition,
			clusterv1.MachineDeploymentDeletingCondition,
		}},
	)
	return patchHelper.Patch(ctx, md, options...)
}

func (r *Reconciler) reconcile(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Reconcile MachineDeployment")

	md := s.machineDeployment
	cluster := s.cluster

	// Reconcile and retrieve the Cluster object.
	if md.Labels == nil {
		md.Labels = make(map[string]string)
	}
	if md.Spec.Selector.MatchLabels == nil {
		md.Spec.Selector.MatchLabels = make(map[string]string)
	}
	if md.Spec.Template.Labels == nil {
		md.Spec.Template.Labels = make(map[string]string)
	}

	md.Labels[clusterv1.ClusterNameLabel] = md.Spec.ClusterName

	// Ensure the MachineDeployment is owned by the Cluster.
	md.SetOwnerReferences(util.EnsureOwnerRef(md.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	if err := r.getTemplatesAndSetOwner(ctx, s); err != nil {
		return err
	}

	if err := r.getAndAdoptMachineSetsForDeployment(ctx, s); err != nil {
		return err
	}

	var anyManagedFieldIssueMitigated bool
	for _, ms := range s.machineSets {
		managedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, ms, machineDeploymentManagerName)
		if err != nil {
			return err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated
	}
	if anyManagedFieldIssueMitigated {
		return nil // No requeue needed, changes will trigger another reconcile.
	}

	// If not already present, add a label specifying the MachineDeployment name to MachineSets.
	// Ensure all required labels exist on the controlled MachineSets.
	// This logic is needed to add the `cluster.x-k8s.io/deployment-name` label to MachineSets
	// which were created before the `cluster.x-k8s.io/deployment-name` label was added
	// to all MachineSets created by a MachineDeployment or if a user manually removed the label.
	for idx := range s.machineSets {
		machineSet := s.machineSets[idx]
		if name, ok := machineSet.Labels[clusterv1.MachineDeploymentNameLabel]; ok && name == md.Name {
			continue
		}

		helper, err := patch.NewHelper(machineSet, r.Client)
		if err != nil {
			return errors.Wrapf(err, "failed to apply %s label to MachineSet %q", clusterv1.MachineDeploymentNameLabel, machineSet.Name)
		}
		machineSet.Labels[clusterv1.MachineDeploymentNameLabel] = md.Name
		if err := helper.Patch(ctx, machineSet); err != nil {
			return errors.Wrapf(err, "failed to apply %s label to MachineSet %q", clusterv1.MachineDeploymentNameLabel, machineSet.Name)
		}
	}

	templateExists := s.infrastructureTemplateExists && (!md.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() || s.bootstrapTemplateExists)

	if ptr.Deref(md.Spec.Paused, false) {
		return r.sync(ctx, md, s.machineSets, s.machines, templateExists)
	}

	if md.Spec.Rollout.Strategy.Type == clusterv1.RollingUpdateMachineDeploymentStrategyType {
		return r.rolloutRollingUpdate(ctx, md, s.machineSets, s.machines, templateExists)
	}

	if md.Spec.Rollout.Strategy.Type == clusterv1.OnDeleteMachineDeploymentStrategyType {
		return r.rolloutOnDelete(ctx, md, s.machineSets, s.machines, templateExists)
	}

	return errors.Errorf("unexpected deployment strategy type: %s", md.Spec.Rollout.Strategy.Type)
}

// createOrUpdateMachineSetsAndSyncMachineDeploymentRevision applies changes identified by the rolloutPlanner to both newMS and oldMSs.
// Note: Both newMS and oldMS include the full intent for the SSA apply call with mandatory labels,
// in place propagated fields, the annotations derived from the MachineDeployment, revision annotations
// and also annotations influencing how to perform scale up/down operations.
// scaleIntents instead are handled separately in the rolloutPlanner and should be applied to MachineSets
// before persisting changes.
// Note: When the newMS has been created by the rollout planner, also wait for the cache to be up to date.
func (r *Reconciler) createOrUpdateMachineSetsAndSyncMachineDeploymentRevision(ctx context.Context, p *rolloutPlanner) error {
	log := ctrl.LoggerFrom(ctx)

	// Note: newMS goes first so in the logs we will have first create/scale up newMS, then scale down oldMSs
	allMSs := append([]*clusterv1.MachineSet{p.newMS}, p.oldMSs...)

	// Get all the diff introduced by the rollout planner.
	// Note: collect all the diff first, so for each change we can add an overview of all the MachineSets
	// in the MachineDeployment, because those info are required to understand why a change happened.
	machineSetsDiff := map[string]machineSetDiff{}
	machineSetsSummary := map[string]string{}
	for _, ms := range allMSs {
		// Update spec.Replicas in the MachineSet.
		if scaleIntent, ok := p.scaleIntents[ms.Name]; ok {
			ms.Spec.Replicas = ptr.To(scaleIntent)
		}

		diff := p.getMachineSetDiff(ms)
		machineSetsDiff[ms.Name] = diff
		machineSetsSummary[ms.Name] = diff.Summary
	}

	// Apply changes to MachineSets.
	for _, ms := range allMSs {
		log := log.WithValues("MachineSet", klog.KObj(ms))
		ctx := ctrl.LoggerInto(ctx, log)

		// Retrieve the diff for the MachineSet.
		// Note: no need to check for diff doesn't exist, all the diff have been computed right above.
		diff := machineSetsDiff[ms.Name]

		// Add to the log kv pairs providing the overview of all the MachineSets computed above.
		// Note: This value should not be added to the context to prevent propagation to other func.
		log = log.WithValues("machineSets", machineSetsSummary)

		if diff.OriginalMS == nil {
			// Create the MachineSet.
			if err := ssa.Patch(ctx, r.Client, machineDeploymentManagerName, ms); err != nil {
				r.recorder.Eventf(p.md, corev1.EventTypeWarning, "FailedCreate", "Failed to create MachineSet %s: %v", klog.KObj(ms), err)
				return errors.Wrapf(err, "failed to create MachineSet %s", klog.KObj(ms))
			}
			if len(p.oldMSs) > 0 {
				log.Info(fmt.Sprintf("MachineSets need rollout: %s", strings.Join(machineSetNames(p.oldMSs), ", ")), "reason", p.createReason)
			}
			log.Info(fmt.Sprintf("MachineSet %s created, it is now the current MachineSet", ms.Name))
			if diff.DesiredReplicas > 0 {
				log.Info(fmt.Sprintf("Scaled up current MachineSet %s from 0 to %d replicas (+%[2]d)", ms.Name, diff.DesiredReplicas))
			}
			r.recorder.Eventf(p.md, corev1.EventTypeNormal, "SuccessfulCreate", "Created MachineSet %s with %d replicas", klog.KObj(ms), diff.DesiredReplicas)

			// Keep trying to get the MachineSet. This will force the cache to update and prevent any future reconciliation of
			// the MachineDeployment to reconcile with an outdated list of MachineSets which could lead to unwanted creation of
			// a duplicate MachineSet.
			if err := clientutil.WaitForObjectsToBeAddedToTheCache(ctx, r.Client, "MachineSet creation", ms); err != nil {
				return err
			}

			continue
		}

		// Add to the log kv pairs providing context and details about changes in this MachineSet (reason, diff)
		// Note: Those values should not be added to the context to prevent propagation to other func.
		statusToLogKeyAndValues := []any{
			"reason", diff.Reason,
			"diff", diff.OtherChanges,
		}
		if len(p.acknowledgedMachineNames) > 0 {
			statusToLogKeyAndValues = append(statusToLogKeyAndValues, "acknowledgedMachines", sortAndJoin(p.acknowledgedMachineNames))
		}
		if len(p.updatingMachineNames) > 0 {
			statusToLogKeyAndValues = append(statusToLogKeyAndValues, "updatingMachines", sortAndJoin(p.updatingMachineNames))
		}
		log = log.WithValues(statusToLogKeyAndValues...)

		err := ssa.Patch(ctx, r.Client, machineDeploymentManagerName, ms, ssa.WithCachingProxy{Cache: r.ssaCache, Original: diff.OriginalMS})
		if err != nil {
			// Note: If we are Applying a MachineSet with UID set and the MachineSet does not exist anymore, the
			// kube-apiserver returns a conflict error.
			if (apierrors.IsConflict(err) || apierrors.IsNotFound(err)) && !ms.DeletionTimestamp.IsZero() {
				continue
			}
			r.recorder.Eventf(p.md, corev1.EventTypeWarning, "FailedUpdate", "Failed to update MachineSet %s: %v", klog.KObj(ms), err)
			return errors.Wrapf(err, "failed to update MachineSet %s", klog.KObj(ms))
		}

		if diff.DesiredReplicas < diff.OriginalReplicas {
			log.Info(fmt.Sprintf("Scaled down %s MachineSet %s from %d to %d replicas (-%d)", diff.Type, ms.Name, diff.OriginalReplicas, diff.DesiredReplicas, diff.OriginalReplicas-diff.DesiredReplicas))
			r.recorder.Eventf(p.md, corev1.EventTypeNormal, "SuccessfulScale", "Scaled down MachineSet %v: %d -> %d", ms.Name, diff.OriginalReplicas, diff.DesiredReplicas)
		}
		if diff.DesiredReplicas > diff.OriginalReplicas {
			log.Info(fmt.Sprintf("Scaled up %s MachineSet %s from %d to %d replicas (+%d)", diff.Type, ms.Name, diff.OriginalReplicas, diff.DesiredReplicas, diff.DesiredReplicas-diff.OriginalReplicas))
			r.recorder.Eventf(p.md, corev1.EventTypeNormal, "SuccessfulScale", "Scaled up MachineSet %v: %d -> %d", ms.Name, diff.OriginalReplicas, diff.DesiredReplicas)
		}
		if diff.DesiredReplicas == diff.OriginalReplicas && diff.OtherChanges != "" {
			log.Info(fmt.Sprintf("Updated %s MachineSet %s", diff.Type, ms.Name))
		}

		// Only wait for cache if the object was changed.
		if diff.OriginalMS.ResourceVersion != ms.ResourceVersion {
			if err := clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "MachineSet update", ms); err != nil {
				return err
			}
		}
	}

	// Surface the revision annotation on the MD level
	if p.md.Annotations == nil {
		p.md.Annotations = make(map[string]string)
	}
	if p.md.Annotations[clusterv1.RevisionAnnotation] != p.revision {
		p.md.Annotations[clusterv1.RevisionAnnotation] = p.revision
	}

	return nil
}

func machineSetNames(machineSets []*clusterv1.MachineSet) []string {
	names := []string{}
	for _, ms := range machineSets {
		names = append(names, ms.Name)
	}
	sort.Strings(names)
	return names
}

func (r *Reconciler) reconcileDelete(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)
	if err := r.getAndAdoptMachineSetsForDeployment(ctx, s); err != nil {
		return err
	}

	// If all the descendant machinesets are deleted, then remove the machinedeployment's finalizer.
	if len(s.machineSets) == 0 {
		controllerutil.RemoveFinalizer(s.machineDeployment, clusterv1.MachineDeploymentFinalizer)
		return nil
	}

	// else delete owned machinesets.
	for _, ms := range s.machineSets {
		if ms.DeletionTimestamp.IsZero() {
			if err := r.Client.Delete(ctx, ms); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete MachineSet %s (MachineDeployment deleting)", klog.KObj(ms))
			}
			// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
			// Also, setting DeletionTimestamp doesn't mean the MachineSet is actually deleted (deletion takes some time).
			log.Info(fmt.Sprintf("MachineSet %s deleting (MachineDeployment deleting)", ms.Name), "MachineSet", klog.KObj(ms))
		}
	}

	log.Info("Waiting for MachineSets to be deleted", "MachineSets", clog.ObjNamesString(s.machineSets))
	return nil
}

// getAndAdoptMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func (r *Reconciler) getAndAdoptMachineSetsForDeployment(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	md := s.machineDeployment

	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &clusterv1.MachineSetList{}
	if err := r.Client.List(ctx, machineSets, client.InNamespace(md.Namespace)); err != nil {
		return err
	}

	selector, err := metav1.LabelSelectorAsSelector(&md.Spec.Selector)
	if err != nil {
		return errors.Wrapf(err, "failed to get MachineSets: failed to compute label selector from MachineDeployment.spec.selector")
	}

	if selector.Empty() {
		return errors.New("failed to get MachineSets: label selector computed from MachineDeployment.spec.selector is empty")
	}

	filtered := make([]*clusterv1.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]
		log := log.WithValues("MachineSet", klog.KObj(ms))
		ctx := ctrl.LoggerInto(ctx, log)

		// Skip this MachineSet unless either selector matches or it has a controller ref pointing to this MachineDeployment
		if !selector.Matches(labels.Set(ms.Labels)) && !metav1.IsControlledBy(ms, md) {
			log.V(4).Info("Skipping MachineSet, label mismatch")
			continue
		}

		// Attempt to adopt MachineSet if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(ms) == nil {
			if err := r.adoptOrphan(ctx, md, ms); err != nil {
				log.Error(err, "Failed to adopt MachineSet into MachineDeployment")
				r.recorder.Eventf(md, corev1.EventTypeWarning, "FailedAdopt", "Failed to adopt MachineSet %q: %v", ms.Name, err)
				continue
			}
			log.Info("Adopted MachineSet into MachineDeployment")
			r.recorder.Eventf(md, corev1.EventTypeNormal, "SuccessfulAdopt", "Adopted MachineSet %q", ms.Name)
		}

		if !metav1.IsControlledBy(ms, md) {
			continue
		}

		filtered = append(filtered, ms)
	}

	s.getAndAdoptMachineSetsForDeploymentSucceeded = true
	s.machineSets = filtered
	return nil
}

// adoptOrphan sets the MachineDeployment as a controller OwnerReference to the MachineSet.
func (r *Reconciler) adoptOrphan(ctx context.Context, deployment *clusterv1.MachineDeployment, machineSet *clusterv1.MachineSet) error {
	patch := client.MergeFrom(machineSet.DeepCopy())
	newRef := *metav1.NewControllerRef(deployment, machineDeploymentKind)
	machineSet.SetOwnerReferences(util.EnsureOwnerRef(machineSet.GetOwnerReferences(), newRef))
	return r.Client.Patch(ctx, machineSet, patch)
}

// getMachineDeploymentsForMachineSet returns a list of MachineDeployments that could potentially match a MachineSet.
func (r *Reconciler) getMachineDeploymentsForMachineSet(ctx context.Context, ms *clusterv1.MachineSet) []*clusterv1.MachineDeployment {
	log := ctrl.LoggerFrom(ctx)

	if len(ms.Labels) == 0 {
		log.V(2).Info("No MachineDeployments found for MachineSet because it has no labels", "MachineSet", klog.KObj(ms))
		return nil
	}

	dList := &clusterv1.MachineDeploymentList{}
	if err := r.Client.List(ctx, dList, client.InNamespace(ms.Namespace)); err != nil {
		log.Error(err, "Failed to list MachineDeployments")
		return nil
	}

	deployments := make([]*clusterv1.MachineDeployment, 0, len(dList.Items))
	for idx := range dList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&dList.Items[idx].Spec.Selector)
		if err != nil {
			continue
		}

		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(ms.Labels)) {
			continue
		}

		deployments = append(deployments, &dList.Items[idx])
	}

	return deployments
}

// MachineSetToDeployments is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for MachineDeployments that might adopt an orphaned MachineSet.
func (r *Reconciler) MachineSetToDeployments(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}

	ms, ok := o.(*clusterv1.MachineSet)
	if !ok {
		panic(fmt.Sprintf("Expected a MachineSet but got a %T", o))
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range ms.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mds := r.getMachineDeploymentsForMachineSet(ctx, ms)
	if len(mds) == 0 {
		return nil
	}

	for _, md := range mds {
		name := client.ObjectKey{Namespace: md.Namespace, Name: md.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

// getTemplatesAndSetOwner reconciles the templates referenced by a MachineDeployment ensuring they are owned by the Cluster.
func (r *Reconciler) getTemplatesAndSetOwner(ctx context.Context, s *scope) error {
	md := s.machineDeployment
	cluster := s.cluster

	// Make sure to reconcile the external infrastructure reference.
	if err := reconcileExternalTemplateReference(ctx, r.Client, cluster, md.Spec.Template.Spec.InfrastructureRef); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		s.infrastructureTemplateNotFound = true
	} else {
		s.infrastructureTemplateExists = true
	}
	// Make sure to reconcile the external bootstrap reference, if any.
	if md.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		if err := reconcileExternalTemplateReference(ctx, r.Client, cluster, md.Spec.Template.Spec.Bootstrap.ConfigRef); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			s.bootstrapTemplateNotFound = true
		} else {
			s.bootstrapTemplateExists = true
		}
	}
	return nil
}

func reconcileExternalTemplateReference(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, ref clusterv1.ContractVersionedObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	obj, err := external.GetObjectFromContractVersionedRef(ctx, c, ref, cluster.Namespace)
	if err != nil {
		return err
	}

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef))

	return patchHelper.Patch(ctx, obj)
}
