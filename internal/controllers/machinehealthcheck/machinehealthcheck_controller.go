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

package machinehealthcheck

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/controllers/machine"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	// Event types.

	// EventRemediationRestricted is emitted in case when machine remediation
	// is restricted by remediation circuit shorting logic.
	EventRemediationRestricted string = "RemediationRestricted"

	maxUnhealthyKeyLog     = "maxUnhealthy"
	unhealthyTargetsKeyLog = "unhealthyTargets"
	unhealthyRangeKeyLog   = "unhealthyRange"
	totalTargetKeyLog      = "totalTarget"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinehealthchecks;machinehealthchecks/status;machinehealthchecks/finalizers,verbs=get;list;watch;update;patch

// Reconciler reconciles a MachineHealthCheck object.
type Reconciler struct {
	Client       client.Client
	ClusterCache clustercache.ClusterCache

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	controller controller.Controller
	recorder   record.EventRecorder

	predicateLog logr.Logger
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ClusterCache == nil {
		return errors.New("Client and ClusterCache must not be nil")
	}

	r.predicateLog = ctrl.LoggerFrom(ctx).WithValues("controller", "machinehealthcheck")
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineHealthCheck{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(r.machineToMachineHealthCheck),
			builder.WithPredicates(predicates.ResourceIsChanged(r.predicateLog)),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), r.predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.clusterToMachineHealthCheck),
			builder.WithPredicates(
				// TODO: should this wait for Cluster.Status.InfrastructureReady similar to Infra Machine resources?
				predicates.All(mgr.GetScheme(), r.predicateLog,
					predicates.ResourceIsChanged(r.predicateLog),
					predicates.ClusterPausedTransitions(mgr.GetScheme(), r.predicateLog),
					predicates.ResourceHasFilterLabel(mgr.GetScheme(), r.predicateLog, r.WatchFilterValue),
				),
			),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("machinehealthcheck", r.clusterToMachineHealthCheck)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("machinehealthcheck-controller")
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the MachineHealthCheck instance
	m := &clusterv1.MachineHealthCheck{}
	if err := r.Client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", klog.KRef(m.Namespace, m.Spec.ClusterName))
	ctx = ctrl.LoggerInto(ctx, log)

	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to fetch Cluster for MachineHealthCheck")
		return ctrl.Result{}, err
	}

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, m); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.RemediationAllowedCondition,
			}},
			patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.MachineHealthCheckRemediationAllowedV1Beta2Condition,
			}},
		}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, m, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Reconcile labels.
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	result, err := r.reconcile(ctx, log, cluster, m)
	if err != nil {
		// Requeue if the reconcile failed because connection to workload cluster was down.
		if errors.Is(err, clustercache.ErrClusterNotConnected) {
			log.V(5).Info("Requeuing because connection to the workload cluster is down")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		// Requeue immediately if any errors occurred
		return ctrl.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) reconcile(ctx context.Context, logger logr.Logger, cluster *clusterv1.Cluster, m *clusterv1.MachineHealthCheck) (ctrl.Result, error) {
	// Ensure the MachineHealthCheck is owned by the Cluster it belongs to
	m.SetOwnerReferences(util.EnsureOwnerRef(m.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	// If the cluster is already initialized, get the remote cluster cache to use as a client.Reader.
	var remoteClient client.Client
	if conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		var err error
		remoteClient, err = r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
		if err != nil {
			logger.Error(err, "Error creating remote cluster cache")
			return ctrl.Result{}, err
		}

		if err := r.watchClusterNodes(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// fetch all targets
	logger.V(3).Info("Finding targets")
	targets, err := r.getTargetsFromMHC(ctx, logger, remoteClient, cluster, m)
	if err != nil {
		logger.Error(err, "Failed to fetch targets from MachineHealthCheck")
		return ctrl.Result{}, err
	}
	totalTargets := len(targets)
	m.Status.ExpectedMachines = int32(totalTargets)
	m.Status.Targets = make([]string, totalTargets)
	for i, t := range targets {
		m.Status.Targets[i] = t.Machine.Name
	}
	// do sort to avoid keep changing m.Status as the returned machines are not in order
	sort.Strings(m.Status.Targets)

	nodeStartupTimeout := m.Spec.NodeStartupTimeout
	if nodeStartupTimeout == nil {
		nodeStartupTimeout = &clusterv1.DefaultNodeStartupTimeout
	}

	// health check all targets and reconcile mhc status
	healthy, unhealthy, nextCheckTimes := r.healthCheckTargets(targets, logger, *nodeStartupTimeout)
	m.Status.CurrentHealthy = int32(len(healthy))

	// check MHC current health against MaxUnhealthy
	remediationAllowed, remediationCount, err := isAllowedRemediation(m)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error checking if remediation is allowed")
	}

	if !remediationAllowed {
		var message string

		if m.Spec.UnhealthyRange == nil {
			logger.V(3).Info(
				"Short-circuiting remediation",
				totalTargetKeyLog, totalTargets,
				maxUnhealthyKeyLog, m.Spec.MaxUnhealthy,
				unhealthyTargetsKeyLog, len(unhealthy),
			)
			message = fmt.Sprintf("Remediation is not allowed, the number of not started or unhealthy machines exceeds maxUnhealthy (total: %v, unhealthy: %v, maxUnhealthy: %v)",
				totalTargets,
				len(unhealthy),
				m.Spec.MaxUnhealthy)
		} else {
			logger.V(3).Info(
				"Short-circuiting remediation",
				totalTargetKeyLog, totalTargets,
				unhealthyRangeKeyLog, *m.Spec.UnhealthyRange,
				unhealthyTargetsKeyLog, len(unhealthy),
			)
			message = fmt.Sprintf("Remediation is not allowed, the number of not started or unhealthy machines does not fall within the range (total: %v, unhealthy: %v, unhealthyRange: %v)",
				totalTargets,
				len(unhealthy),
				*m.Spec.UnhealthyRange)
		}

		// Remediation not allowed, the number of not started or unhealthy machines either exceeds maxUnhealthy (or) not within unhealthyRange
		m.Status.RemediationsAllowed = 0
		conditions.Set(m, &clusterv1.Condition{
			Type:     clusterv1.RemediationAllowedCondition,
			Status:   corev1.ConditionFalse,
			Severity: clusterv1.ConditionSeverityWarning,
			Reason:   clusterv1.TooManyUnhealthyReason,
			Message:  message,
		})

		v1beta2conditions.Set(m, metav1.Condition{
			Type:    clusterv1.MachineHealthCheckRemediationAllowedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineHealthCheckTooManyUnhealthyV1Beta2Reason,
			Message: message,
		})

		// If there are no unhealthy target, skip publishing the `RemediationRestricted` event to avoid misleading.
		if len(unhealthy) != 0 {
			r.recorder.Event(
				m,
				corev1.EventTypeWarning,
				EventRemediationRestricted,
				message,
			)
		}
		errList := []error{}
		for _, t := range append(healthy, unhealthy...) {
			patchOpts := []patch.Option{
				patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
					clusterv1.MachineHealthCheckSucceededCondition,
					// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
				}},
				patch.WithOwnedV1Beta2Conditions{Conditions: []string{
					clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
					// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
					// (Same for ExternallyRemediated condition)
				}},
			}
			if err := t.patchHelper.Patch(ctx, t.Machine, patchOpts...); err != nil {
				errList = append(errList, errors.Wrapf(err, "failed to patch machine status for machine: %s/%s", t.Machine.Namespace, t.Machine.Name))
				continue
			}
		}
		if len(errList) > 0 {
			return ctrl.Result{}, kerrors.NewAggregate(errList)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	if m.Spec.UnhealthyRange == nil {
		logger.V(3).Info(
			"Remediations are allowed",
			totalTargetKeyLog, totalTargets,
			maxUnhealthyKeyLog, m.Spec.MaxUnhealthy,
			unhealthyTargetsKeyLog, len(unhealthy),
		)
	} else {
		logger.V(3).Info(
			"Remediations are allowed",
			totalTargetKeyLog, totalTargets,
			unhealthyRangeKeyLog, *m.Spec.UnhealthyRange,
			unhealthyTargetsKeyLog, len(unhealthy),
		)
	}

	// Remediation is allowed so unhealthyMachineCount is within unhealthyRange (or) maxUnhealthy - unhealthyMachineCount >= 0
	m.Status.RemediationsAllowed = remediationCount
	conditions.MarkTrue(m, clusterv1.RemediationAllowedCondition)

	v1beta2conditions.Set(m, metav1.Condition{
		Type:   clusterv1.MachineHealthCheckRemediationAllowedV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineHealthCheckRemediationAllowedV1Beta2Reason,
	})

	errList := r.patchUnhealthyTargets(ctx, logger, unhealthy, cluster, m)
	errList = append(errList, r.patchHealthyTargets(ctx, logger, healthy, m)...)

	// handle update errors
	if len(errList) > 0 {
		logger.V(3).Info("Error(s) marking machine, requeuing")
		return reconcile.Result{}, kerrors.NewAggregate(errList)
	}

	if minNextCheck := minDuration(nextCheckTimes); minNextCheck > 0 {
		logger.V(3).Info("Some targets might go unhealthy. Ensuring a requeue happens", "requeueAfter", minNextCheck.Truncate(time.Second).String())
		return ctrl.Result{RequeueAfter: minNextCheck}, nil
	}

	logger.V(3).Info("No more targets meet unhealthy criteria")

	return ctrl.Result{}, nil
}

// patchHealthyTargets patches healthy machines with MachineHealthCheckSucceededCondition.
func (r *Reconciler) patchHealthyTargets(ctx context.Context, logger logr.Logger, healthy []healthCheckTarget, m *clusterv1.MachineHealthCheck) []error {
	errList := []error{}
	for _, t := range healthy {
		if m.Spec.RemediationTemplate != nil {
			// Get remediation request object
			obj, err := r.getExternalRemediationRequest(ctx, m, t.Machine.Name)
			if err != nil {
				if !apierrors.IsNotFound(errors.Cause(err)) {
					wrappedErr := errors.Wrapf(err, "failed to fetch remediation request for machine %q in namespace %q within cluster %q", t.Machine.Name, t.Machine.Namespace, t.Machine.Spec.ClusterName)
					errList = append(errList, wrappedErr)
				}
				continue
			}
			// Check that obj has no DeletionTimestamp to avoid hot loop
			if obj.GetDeletionTimestamp() == nil {
				// Issue a delete for remediation request.
				if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
					errList = append(errList, errors.Wrapf(err, "failed to delete %v %q for Machine %q", obj.GroupVersionKind(), obj.GetName(), t.Machine.Name))
					continue
				}
			}
		}

		patchOpts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineHealthCheckSucceededCondition,
				// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
			}},
			patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
				// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
				// (Same for ExternallyRemediated condition)
			}},
		}
		if err := t.patchHelper.Patch(ctx, t.Machine, patchOpts...); err != nil {
			logger.Error(err, "failed to patch healthy machine status for machine", "Machine", klog.KObj(t.Machine))
			errList = append(errList, errors.Wrapf(err, "failed to patch healthy machine status for machine: %s/%s", t.Machine.Namespace, t.Machine.Name))
		}
	}
	return errList
}

// patchUnhealthyTargets patches machines with MachineOwnerRemediatedCondition for remediation.
func (r *Reconciler) patchUnhealthyTargets(ctx context.Context, logger logr.Logger, unhealthy []healthCheckTarget, cluster *clusterv1.Cluster, m *clusterv1.MachineHealthCheck) []error {
	// mark for remediation
	errList := []error{}
	for _, t := range unhealthy {
		condition := conditions.Get(t.Machine, clusterv1.MachineHealthCheckSucceededCondition)

		if annotations.IsPaused(cluster, t.Machine) {
			logger.Info("Machine has failed health check, but machine is paused so skipping remediation", "target", t.string(), "reason", condition.Reason, "message", condition.Message)
		} else {
			if m.Spec.RemediationTemplate != nil {
				// If external remediation request already exists,
				// return early
				if r.externalRemediationRequestExists(ctx, m, t.Machine.Name) {
					return errList
				}

				cloneOwnerRef := &metav1.OwnerReference{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       t.Machine.Name,
					UID:        t.Machine.UID,
				}

				from, err := external.Get(ctx, r.Client, m.Spec.RemediationTemplate)
				if err != nil {
					conditions.MarkFalse(m, clusterv1.ExternalRemediationTemplateAvailableCondition, clusterv1.ExternalRemediationTemplateNotFoundReason, clusterv1.ConditionSeverityError, err.Error())

					v1beta2conditions.Set(t.Machine, metav1.Condition{
						Type:    clusterv1.MachineExternallyRemediatedV1Beta2Condition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineExternallyRemediatedRemediationTemplateNotFoundV1Beta2Reason,
						Message: fmt.Sprintf("Error retrieving remediation template %s %s", m.Spec.RemediationTemplate.Kind, klog.KRef(m.Spec.RemediationTemplate.Namespace, m.Spec.RemediationTemplate.Name)),
					})
					errList = append(errList, errors.Wrapf(err, "error retrieving remediation template %v %q for machine %q in namespace %q within cluster %q", m.Spec.RemediationTemplate.GroupVersionKind(), m.Spec.RemediationTemplate.Name, t.Machine.Name, t.Machine.Namespace, m.Spec.ClusterName))
					return errList
				}

				generateTemplateInput := &external.GenerateTemplateInput{
					Template:    from,
					TemplateRef: m.Spec.RemediationTemplate,
					Namespace:   t.Machine.Namespace,
					ClusterName: t.Machine.Spec.ClusterName,
					OwnerRef:    cloneOwnerRef,
				}
				to, err := external.GenerateTemplate(generateTemplateInput)
				if err != nil {
					errList = append(errList, errors.Wrapf(err, "failed to create template for remediation request %v %q for machine %q in namespace %q within cluster %q", m.Spec.RemediationTemplate.GroupVersionKind(), m.Spec.RemediationTemplate.Name, t.Machine.Name, t.Machine.Namespace, m.Spec.ClusterName))
					return errList
				}

				// Set the Remediation Request to match the Machine name, the name is used to
				// guarantee uniqueness between runs. A Machine should only ever have a single
				// remediation object of a specific GVK created.
				//
				// NOTE: This doesn't guarantee uniqueness across different MHC objects watching
				// the same Machine, users are in charge of setting health checks and remediation properly.
				to.SetName(t.Machine.Name)

				logger.Info("Target has failed health check, creating an external remediation request", "remediation request name", to.GetName(), "target", t.string(), "reason", condition.Reason, "message", condition.Message)
				// Create the external clone.
				if err := r.Client.Create(ctx, to); err != nil {
					conditions.MarkFalse(m, clusterv1.ExternalRemediationRequestAvailableCondition, clusterv1.ExternalRemediationRequestCreationFailedReason, clusterv1.ConditionSeverityError, err.Error())

					v1beta2conditions.Set(t.Machine, metav1.Condition{
						Type:    clusterv1.MachineExternallyRemediatedV1Beta2Condition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineExternallyRemediatedRemediationRequestCreationFailedV1Beta2Reason,
						Message: "Please check controller logs for errors",
					})
					errList = append(errList, errors.Wrapf(err, "error creating remediation request for machine %q in namespace %q within cluster %q", t.Machine.Name, t.Machine.Namespace, t.Machine.Spec.ClusterName))
					return errList
				}

				v1beta2conditions.Set(t.Machine, metav1.Condition{
					Type:   clusterv1.MachineExternallyRemediatedV1Beta2Condition,
					Status: metav1.ConditionFalse,
					Reason: clusterv1.MachineExternallyRemediatedWaitingForRemediationV1Beta2Reason,
				})
			} else if t.Machine.DeletionTimestamp.IsZero() { // Only setting the OwnerRemediated conditions when machine is not already in deletion.
				logger.Info("Target has failed health check, marking for remediation", "target", t.string(), "reason", condition.Reason, "message", condition.Message)
				// NOTE: MHC is responsible for creating MachineOwnerRemediatedCondition if missing or to trigger another remediation if the previous one is completed;
				// instead, if a remediation is in already progress, the remediation owner is responsible for completing the process and MHC should not overwrite the condition.
				if !conditions.Has(t.Machine, clusterv1.MachineOwnerRemediatedCondition) || conditions.IsTrue(t.Machine, clusterv1.MachineOwnerRemediatedCondition) {
					conditions.MarkFalse(t.Machine, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "")
				}

				if ownerRemediatedCondition := v1beta2conditions.Get(t.Machine, clusterv1.MachineOwnerRemediatedV1Beta2Condition); ownerRemediatedCondition == nil || ownerRemediatedCondition.Status == metav1.ConditionTrue {
					v1beta2conditions.Set(t.Machine, metav1.Condition{
						Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
						Message: "Waiting for remediation",
					})
				}
			}
		}

		patchOpts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineHealthCheckSucceededCondition,
				// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
			}},
			patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
				// Note: intentionally leaving out OwnerRemediated condition which is mostly controlled by the owner.
				// (Same for ExternallyRemediated condition)
			}},
		}
		if err := t.patchHelper.Patch(ctx, t.Machine, patchOpts...); err != nil {
			errList = append(errList, errors.Wrapf(err, "failed to patch unhealthy machine status for machine: %s/%s", t.Machine.Namespace, t.Machine.Name))
			continue
		}
		r.recorder.Eventf(
			t.Machine,
			corev1.EventTypeNormal,
			EventMachineMarkedUnhealthy,
			"Machine %v has been marked as unhealthy",
			t.string(),
		)
	}
	return errList
}

// clusterToMachineHealthCheck maps events from Cluster objects to
// MachineHealthCheck objects that belong to the Cluster.
func (r *Reconciler) clusterToMachineHealthCheck(ctx context.Context, o client.Object) []reconcile.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster, got %T", o))
	}

	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := r.Client.List(
		ctx,
		mhcList,
		client.InNamespace(c.Namespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: c.Name},
	); err != nil {
		return nil
	}

	// This list should only contain MachineHealthChecks which belong to the given Cluster
	requests := []reconcile.Request{}
	for _, mhc := range mhcList.Items {
		key := types.NamespacedName{Namespace: mhc.Namespace, Name: mhc.Name}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}

// machineToMachineHealthCheck maps events from Machine objects to
// MachineHealthCheck objects that monitor the given machine.
func (r *Reconciler) machineToMachineHealthCheck(ctx context.Context, o client.Object) []reconcile.Request {
	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine, got %T", o))
	}

	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := r.Client.List(
		ctx,
		mhcList,
		client.InNamespace(m.Namespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: m.Spec.ClusterName},
	); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for k := range mhcList.Items {
		mhc := &mhcList.Items[k]
		if machine.HasMatchingLabels(mhc.Spec.Selector, m.Labels) {
			key := util.ObjectKey(mhc)
			requests = append(requests, reconcile.Request{NamespacedName: key})
		}
	}
	return requests
}

func (r *Reconciler) nodeToMachineHealthCheck(ctx context.Context, o client.Object) []reconcile.Request {
	node, ok := o.(*corev1.Node)
	if !ok {
		panic(fmt.Sprintf("Expected a corev1.Node, got %T", o))
	}

	machine, err := getMachineFromNode(ctx, r.Client, node.Name)
	if machine == nil || err != nil {
		return nil
	}

	return r.machineToMachineHealthCheck(ctx, machine)
}

func (r *Reconciler) watchClusterNodes(ctx context.Context, cluster *clusterv1.Cluster) error {
	return r.ClusterCache.Watch(ctx, util.ObjectKey(cluster), clustercache.NewWatcher(clustercache.WatcherOptions{
		Name:         "machinehealthcheck-watchClusterNodes",
		Watcher:      r.controller,
		Kind:         &corev1.Node{},
		EventHandler: handler.EnqueueRequestsFromMapFunc(r.nodeToMachineHealthCheck),
		Predicates:   []predicate.TypedPredicate[client.Object]{predicates.TypedResourceIsChanged[client.Object](r.predicateLog)},
	}))
}

// getMachineFromNode retrieves the machine with a nodeRef to nodeName
// There should at most one machine with a given nodeRef, returns an error otherwise.
func getMachineFromNode(ctx context.Context, c client.Client, nodeName string) (*clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}
	if err := c.List(
		ctx,
		machineList,
		client.MatchingFields{index.MachineNodeNameField: nodeName},
	); err != nil {
		return nil, errors.Wrap(err, "failed getting machine list")
	}
	// TODO(vincepri): Remove this loop once controller runtime fake client supports
	// adding indexes on objects.
	items := []*clusterv1.Machine{}
	for i := range machineList.Items {
		machine := &machineList.Items[i]
		if machine.Status.NodeRef != nil && machine.Status.NodeRef.Name == nodeName {
			items = append(items, machine)
		}
	}
	if len(items) != 1 {
		return nil, errors.Errorf("expecting one machine for node %v, got %v", nodeName, machineNames(items))
	}
	return items[0], nil
}

func machineNames(machines []*clusterv1.Machine) []string {
	result := make([]string, 0, len(machines))
	for _, m := range machines {
		result = append(result, m.Name)
	}
	return result
}

// isAllowedRemediation checks the value of the MaxUnhealthy field to determine
// returns whether remediation should be allowed or not, the remediation count, and error if any.
func isAllowedRemediation(mhc *clusterv1.MachineHealthCheck) (bool, int32, error) {
	var remediationAllowed bool
	var remediationCount int32
	if mhc.Spec.UnhealthyRange != nil {
		minVal, maxVal, err := getUnhealthyRange(mhc)
		if err != nil {
			return false, 0, err
		}
		unhealthyMachineCount := unhealthyMachineCount(mhc)
		remediationAllowed = unhealthyMachineCount >= minVal && unhealthyMachineCount <= maxVal
		remediationCount = int32(maxVal - unhealthyMachineCount)
		return remediationAllowed, remediationCount, nil
	}

	maxUnhealthy, err := getMaxUnhealthy(mhc)
	if err != nil {
		return false, 0, err
	}

	// Remediation is not allowed if unhealthy is above maxUnhealthy
	unhealthyMachineCount := unhealthyMachineCount(mhc)
	remediationAllowed = unhealthyMachineCount <= maxUnhealthy
	remediationCount = int32(maxUnhealthy - unhealthyMachineCount)
	return remediationAllowed, remediationCount, nil
}

// getUnhealthyRange parses an integer range and returns the min and max values
// Eg. [2-5] will return (2,5,nil).
func getUnhealthyRange(mhc *clusterv1.MachineHealthCheck) (int, int, error) {
	// remove '[' and ']'
	unhealthyRange := (*(mhc.Spec.UnhealthyRange))[1 : len(*mhc.Spec.UnhealthyRange)-1]

	parts := strings.Split(unhealthyRange, "-")

	minVal, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return 0, 0, err
	}

	maxVal, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, err
	}

	if maxVal < minVal {
		return 0, 0, errors.Errorf("max value %d cannot be less than min value %d for unhealthyRange", maxVal, minVal)
	}

	return int(minVal), int(maxVal), nil
}

func getMaxUnhealthy(mhc *clusterv1.MachineHealthCheck) (int, error) {
	if mhc.Spec.MaxUnhealthy == nil {
		return 0, errors.New("spec.maxUnhealthy must be set")
	}
	maxUnhealthy, err := intstr.GetScaledValueFromIntOrPercent(mhc.Spec.MaxUnhealthy, int(mhc.Status.ExpectedMachines), false)
	if err != nil {
		return 0, err
	}
	return maxUnhealthy, nil
}

// unhealthyMachineCount calculates the number of presently unhealthy or missing machines
// ie the delta between the expected number of machines and the current number deemed healthy.
func unhealthyMachineCount(mhc *clusterv1.MachineHealthCheck) int {
	return int(mhc.Status.ExpectedMachines - mhc.Status.CurrentHealthy)
}

// getExternalRemediationRequest gets reference to External Remediation Request, unstructured object.
func (r *Reconciler) getExternalRemediationRequest(ctx context.Context, m *clusterv1.MachineHealthCheck, machineName string) (*unstructured.Unstructured, error) {
	remediationRef := &corev1.ObjectReference{
		APIVersion: m.Spec.RemediationTemplate.APIVersion,
		Kind:       strings.TrimSuffix(m.Spec.RemediationTemplate.Kind, clusterv1.TemplateSuffix),
		Name:       machineName,
		Namespace:  m.Namespace,
	}
	remediationReq, err := external.Get(ctx, r.Client, remediationRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve external remediation request object")
	}
	return remediationReq, nil
}

// externalRemediationRequestExists checks if the External Remediation Request is created
// for the machine.
func (r *Reconciler) externalRemediationRequestExists(ctx context.Context, m *clusterv1.MachineHealthCheck, machineName string) bool {
	remediationReq, err := r.getExternalRemediationRequest(ctx, m, machineName)
	if err != nil {
		return false
	}
	return remediationReq != nil
}
