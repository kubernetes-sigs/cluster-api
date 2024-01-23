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
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var (
	// machineDeploymentKind contains the schema.GroupVersionKind for the MachineDeployment type.
	machineDeploymentKind = clusterv1.GroupVersion.WithKind("MachineDeployment")
)

// machineDeploymentManagerName is the manager name used for Server-Side-Apply (SSA) operations
// in the MachineDeployment controller.
const machineDeploymentManagerName = "capi-machinedeployment"

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/status;machinedeployments/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconciler reconciles a MachineDeployment object.
type Reconciler struct {
	Client                    client.Client
	UnstructuredCachingClient client.Client
	APIReader                 client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	recorder record.EventRecorder
	ssaCache ssa.Cache
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToMachineDeployments, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineDeploymentList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		Owns(&clusterv1.MachineSet{}).
		Watches(
			&clusterv1.MachineSet{},
			handler.EnqueueRequestsFromMapFunc(r.MachineSetToDeployments),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachineDeployments),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(ctrl.LoggerFrom(ctx),
					predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
				),
			),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor("machinedeployment-controller")
	r.ssaCache = ssa.NewCache()
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
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

	cluster, err := util.GetClusterByName(ctx, r.Client, deployment.Namespace, deployment.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, deployment) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(deployment, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
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

	// Ignore deleted MachineDeployments, this can happen when foregroundDeletion
	// is enabled
	if !deployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	err = r.reconcile(ctx, cluster, deployment)
	if err != nil {
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return ctrl.Result{}, err
}

func patchMachineDeployment(ctx context.Context, patchHelper *patch.Helper, md *clusterv1.MachineDeployment, options ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(md,
		conditions.WithConditions(
			clusterv1.MachineDeploymentAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	options = append(options,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			clusterv1.MachineDeploymentAvailableCondition,
		}},
	)
	return patchHelper.Patch(ctx, md, options...)
}

func (r *Reconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, md *clusterv1.MachineDeployment) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Reconcile MachineDeployment")

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

	// Make sure to reconcile the external infrastructure reference.
	if err := reconcileExternalTemplateReference(ctx, r.UnstructuredCachingClient, cluster, &md.Spec.Template.Spec.InfrastructureRef); err != nil {
		return err
	}
	// Make sure to reconcile the external bootstrap reference, if any.
	if md.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		if err := reconcileExternalTemplateReference(ctx, r.UnstructuredCachingClient, cluster, md.Spec.Template.Spec.Bootstrap.ConfigRef); err != nil {
			return err
		}
	}

	msList, err := r.getMachineSetsForDeployment(ctx, md)
	if err != nil {
		return err
	}

	// If not already present, add a label specifying the MachineDeployment name to MachineSets.
	// Ensure all required labels exist on the controlled MachineSets.
	// This logic is needed to add the `cluster.x-k8s.io/deployment-name` label to MachineSets
	// which were created before the `cluster.x-k8s.io/deployment-name` label was added
	// to all MachineSets created by a MachineDeployment or if a user manually removed the label.
	for idx := range msList {
		machineSet := msList[idx]
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

	// Loop over all MachineSets and cleanup managed fields.
	// We do this so that MachineSets that were created/patched before (< v1.4.0) the controller adopted
	// Server-Side-Apply (SSA) can also work with SSA. Otherwise, fields would be co-owned by our "old" "manager" and
	// "capi-machinedeployment" and then we would not be able to e.g. drop labels and annotations.
	// Note: We are cleaning up managed fields for all MachineSets, so we're able to remove this code in a few
	// Cluster API releases. If we do this only for selected MachineSets, we would have to keep this code forever.
	for idx := range msList {
		machineSet := msList[idx]
		if err := ssa.CleanUpManagedFieldsForSSAAdoption(ctx, r.Client, machineSet, machineDeploymentManagerName); err != nil {
			return errors.Wrapf(err, "failed to clean up managedFields of MachineSet %s", klog.KObj(machineSet))
		}
	}

	if md.Spec.Paused {
		return r.sync(ctx, md, msList)
	}

	if md.Spec.Strategy == nil {
		return errors.Errorf("missing MachineDeployment strategy")
	}

	if md.Spec.Strategy.Type == clusterv1.RollingUpdateMachineDeploymentStrategyType {
		if md.Spec.Strategy.RollingUpdate == nil {
			return errors.Errorf("missing MachineDeployment settings for strategy type: %s", md.Spec.Strategy.Type)
		}
		return r.rolloutRolling(ctx, md, msList)
	}

	if md.Spec.Strategy.Type == clusterv1.OnDeleteMachineDeploymentStrategyType {
		return r.rolloutOnDelete(ctx, md, msList)
	}

	return errors.Errorf("unexpected deployment strategy type: %s", md.Spec.Strategy.Type)
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func (r *Reconciler) getMachineSetsForDeployment(ctx context.Context, md *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	log := ctrl.LoggerFrom(ctx)

	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &clusterv1.MachineSetList{}
	if err := r.Client.List(ctx, machineSets, client.InNamespace(md.Namespace)); err != nil {
		return nil, err
	}

	filtered := make([]*clusterv1.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]
		log.WithValues("MachineSet", klog.KObj(ms))
		selector, err := metav1.LabelSelectorAsSelector(&md.Spec.Selector)
		if err != nil {
			log.Error(err, "Skipping MachineSet, failed to get label selector from spec selector")
			continue
		}

		// If a MachineDeployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			log.Info("Skipping MachineSet as the selector is empty")
			continue
		}

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

	return filtered, nil
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
	for _, ref := range ms.ObjectMeta.GetOwnerReferences() {
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

func reconcileExternalTemplateReference(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, c, ref); err != nil {
		return err
	}

	obj, err := external.Get(ctx, c, ref, cluster.Namespace)
	if err != nil {
		return err
	}

	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	return patchHelper.Patch(ctx, obj)
}
