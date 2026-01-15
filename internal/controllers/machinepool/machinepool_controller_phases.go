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

package machinepool

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/internal/remediation"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

func (r *Reconciler) reconcilePhase(mp *clusterv1.MachinePool) {
	// Set the phase to "pending" if nil.
	if mp.Status.Phase == "" {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if ptr.Deref(mp.Status.Initialization.BootstrapDataSecretCreated, false) && !ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if len(mp.Status.NodeRefs) != 0 {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseProvisioned)
	}

	// Set the phase to "running" if the number of ready replicas is equal to desired replicas.
	// TODO (v1beta2) Use new replica counters
	readyReplicas := int32(0)
	if mp.Status.Deprecated != nil && mp.Status.Deprecated.V1Beta1 != nil {
		readyReplicas = mp.Status.Deprecated.V1Beta1.ReadyReplicas
	}
	if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) && mp.Spec.Replicas != nil && *mp.Spec.Replicas == readyReplicas {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseRunning)
	}

	// Set the appropriate phase in response to the MachinePool replica count being greater than the observed infrastructure replicas.
	if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) && mp.Spec.Replicas != nil && *mp.Spec.Replicas > readyReplicas {
		// If we are being managed by an external autoscaler and can't predict scaling direction, set to "Scaling".
		if annotations.ReplicasManagedByExternalAutoscaler(mp) {
			mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseScaling)
		} else {
			// Set the phase to "ScalingUp" if we are actively scaling the infrastructure out.
			mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseScalingUp)
		}
	}

	// Set the appropriate phase in response to the MachinePool replica count being less than the observed infrastructure replicas.
	if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) && mp.Spec.Replicas != nil && *mp.Spec.Replicas < readyReplicas {
		// If we are being managed by an external autoscaler and can't predict scaling direction, set to "Scaling".
		if annotations.ReplicasManagedByExternalAutoscaler(mp) {
			mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseScaling)
		} else {
			// Set the phase to "ScalingDown" if we are actively scaling the infrastructure in.
			mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseScalingDown)
		}
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !mp.DeletionTimestamp.IsZero() {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a MachinePool.
func (r *Reconciler) reconcileExternal(ctx context.Context, m *clusterv1.MachinePool, ref clusterv1.ContractVersionedObjectReference) (external.ReconcileOutput, error) {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return external.ReconcileOutput{}, errors.Wrapf(err, "could not find %s %s for MachinePool %s, requeuing",
				ref.Kind, klog.KRef(m.Namespace, ref.Name), klog.KObj(m))
		}
		return external.ReconcileOutput{}, err
	}

	// Ensure we add a watch to the external object, if there isn't one already.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &clusterv1.MachinePool{}), predicates.ResourceIsChanged(r.Client.Scheme(), *r.externalTracker.PredicateLogger)); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set external object ControllerReference to the MachinePool.
	if err := controllerutil.SetControllerReference(m, obj, r.Client.Scheme()); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set the Cluster label.
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName
	obj.SetLabels(labels)

	// Always attempt to Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return external.ReconcileOutput{}, err
	}
	if failureReason != "" {
		machineStatusFailure := capierrors.MachinePoolStatusFailure(failureReason)
		if m.Status.Deprecated != nil {
			m.Status.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
		}
		if m.Status.Deprecated.V1Beta1 != nil {
			m.Status.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
		}
		m.Status.Deprecated.V1Beta1.FailureReason = &machineStatusFailure
	}
	if failureMessage != "" {
		if m.Status.Deprecated != nil {
			m.Status.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
		}
		if m.Status.Deprecated.V1Beta1 != nil {
			m.Status.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
		}
		m.Status.Deprecated.V1Beta1.FailureMessage = ptr.To(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return external.ReconcileOutput{Result: obj}, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a MachinePool.
func (r *Reconciler) reconcileBootstrap(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	m := s.machinePool
	// Call generic external reconciler if we have an external reference.
	var bootstrapConfig *unstructured.Unstructured
	if m.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		bootstrapReconcileResult, err := r.reconcileExternal(ctx, m, m.Spec.Template.Spec.Bootstrap.ConfigRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		bootstrapConfig = bootstrapReconcileResult.Result

		// If the bootstrap config is being deleted, return early.
		if !bootstrapConfig.GetDeletionTimestamp().IsZero() {
			return ctrl.Result{}, nil
		}

		// Determine contract version used by the BootstrapConfig.
		contractVersion, err := contract.GetContractVersion(ctx, r.Client, bootstrapConfig.GroupVersionKind().GroupKind())
		if err != nil {
			return ctrl.Result{}, err
		}

		// Determine if the data secret was created.
		var dataSecretCreated bool
		if dataSecretCreatedPtr, err := contract.Bootstrap().DataSecretCreated(contractVersion).Get(bootstrapConfig); err != nil {
			if !errors.Is(err, contract.ErrFieldNotFound) {
				return ctrl.Result{}, err
			}
		} else {
			dataSecretCreated = *dataSecretCreatedPtr
		}

		// Report a summary of current status of the bootstrap object defined for this machine pool.
		v1beta1conditions.SetMirror(m, clusterv1.BootstrapReadyV1Beta1Condition,
			v1beta1conditions.UnstructuredGetter(bootstrapConfig),
			v1beta1conditions.WithFallbackValue(dataSecretCreated, clusterv1.WaitingForDataSecretFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, ""),
		)

		if !dataSecretCreated {
			log.Info("Waiting for bootstrap provider to generate data secret and report status.ready", bootstrapConfig.GetKind(), klog.KObj(bootstrapConfig))
			m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(dataSecretCreated)
			return ctrl.Result{}, nil
		}

		// Get and set the name of the secret containing the bootstrap data.
		secretName, err := contract.Bootstrap().DataSecretName().Get(bootstrapConfig)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		} else if secretName == nil {
			return ctrl.Result{}, errors.Errorf("retrieved empty dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		}

		m.Spec.Template.Spec.Bootstrap.DataSecretName = secretName
		m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
		return ctrl.Result{}, nil
	}

	// If dataSecretName is set without a ConfigRef, this means the user brought their own bootstrap data.
	if m.Spec.Template.Spec.Bootstrap.DataSecretName != nil {
		m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
		v1beta1conditions.MarkTrue(m, clusterv1.BootstrapReadyV1Beta1Condition)
		return ctrl.Result{}, nil
	}

	// This should never happen because the MachinePool webhook would not allow neither ConfigRef nor DataSecretName to be set.
	return ctrl.Result{}, errors.Errorf("neither .spec.bootstrap.configRef nor .spec.bootstrap.dataSecretName are set for MachinePool %q in namespace %q", m.Name, m.Namespace)
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a MachinePool.
func (r *Reconciler) reconcileInfrastructure(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	mp := s.machinePool
	// Call generic external reconciler.
	infraReconcileResult, err := r.reconcileExternal(ctx, mp, mp.Spec.Template.Spec.InfrastructureRef)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			log.Error(err, "infrastructure reference could not be found")
			if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
				// Infra object went missing after the machine pool was up and running
				log.Error(err, "infrastructure reference has been deleted after being ready, setting failure state")
				if mp.Status.Deprecated == nil {
					mp.Status.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
				}
				if mp.Status.Deprecated.V1Beta1 == nil {
					mp.Status.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
				}
				mp.Status.Deprecated.V1Beta1.FailureReason = ptr.To(capierrors.InvalidConfigurationMachinePoolError)
				mp.Status.Deprecated.V1Beta1.FailureMessage = ptr.To(fmt.Sprintf("MachinePool infrastructure resource %s %s has been deleted after being ready",
					mp.Spec.Template.Spec.InfrastructureRef.Kind, klog.KRef(mp.Namespace, mp.Spec.Template.Spec.InfrastructureRef.Name)))
			}
			v1beta1conditions.MarkFalse(mp, clusterv1.InfrastructureReadyV1Beta1Condition, clusterv1.IncorrectExternalRefV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", fmt.Sprintf("could not find infra reference of kind %s with name %s", mp.Spec.Template.Spec.InfrastructureRef.Kind, mp.Spec.Template.Spec.InfrastructureRef.Name))
		}
		return ctrl.Result{}, err
	}
	infraConfig := infraReconcileResult.Result
	s.infraMachinePool = infraConfig

	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	mp.Status.Initialization.InfrastructureProvisioned = ptr.To(ready)

	// Report a summary of current status of the infrastructure object defined for this machine pool.
	v1beta1conditions.SetMirror(mp, clusterv1.InfrastructureReadyV1Beta1Condition,
		v1beta1conditions.UnstructuredGetter(infraConfig),
		v1beta1conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, ""),
	)

	clusterClient, err := r.ClusterCache.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	var getNodeRefsErr error
	// Get the nodeRefsMap from the cluster.
	s.nodeRefMap, getNodeRefsErr = r.getNodeRefMap(ctx, clusterClient)

	res := ctrl.Result{}

	reconcileMachinesRes, err := r.reconcileMachines(ctx, s, infraConfig)
	if err != nil || getNodeRefsErr != nil {
		return ctrl.Result{}, kerrors.NewAggregate([]error{errors.Wrapf(err, "failed to reconcile Machines for MachinePool %s", klog.KObj(mp)), errors.Wrapf(getNodeRefsErr, "failed to get nodeRefs for MachinePool %s", klog.KObj(mp))})
	}
	res = util.LowestNonZeroResult(res, reconcileMachinesRes)

	if !ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
		log.Info("Infrastructure provider is not yet ready", infraConfig.GetKind(), klog.KObj(infraConfig))
		return res, nil
	}

	var providerIDList []string
	// Get Spec.ProviderIDList from the infrastructure provider.
	if err := util.UnstructuredUnmarshalField(infraConfig, &providerIDList, "spec", "providerIDList"); err != nil && !errors.Is(err, util.ErrUnstructuredFieldNotFound) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve data from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	// Get and set Status.Replicas from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &mp.Status.Replicas, "status", "replicas")
	if err != nil {
		if !errors.Is(err, util.ErrUnstructuredFieldNotFound) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve replicas from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
		}
	}

	if len(providerIDList) == 0 && ptr.Deref(mp.Status.Replicas, 0) != 0 {
		log.Info("Retrieved empty spec.providerIDList from infrastructure provider but status.replicas is not zero.", "replicas", ptr.Deref(mp.Status.Replicas, 0))
		return ctrl.Result{}, nil
	}

	if !reflect.DeepEqual(mp.Spec.ProviderIDList, providerIDList) {
		mp.Spec.ProviderIDList = providerIDList
		if mp.Status.Deprecated == nil {
			mp.Status.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
		}
		if mp.Status.Deprecated.V1Beta1 == nil {
			mp.Status.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
		}
		mp.Status.Deprecated.V1Beta1.ReadyReplicas = 0
		mp.Status.Deprecated.V1Beta1.AvailableReplicas = 0
		mp.Status.Deprecated.V1Beta1.UnavailableReplicas = ptr.Deref(mp.Status.Replicas, 0)
	}

	return res, nil
}

// reconcileMachines reconciles Machines associated with a MachinePool.
//
// Note: In the case of MachinePools the machines are created in order to surface in CAPI what exists in the
// infrastructure while instead on MachineDeployments, machines are created in CAPI first and then the
// infrastructure is created accordingly.
// Note: When supported by the cloud provider implementation of the MachinePool, machines will provide a means to interact
// with the corresponding infrastructure (e.g. delete a specific machine in case MachineHealthCheck detects it is unhealthy).
func (r *Reconciler) reconcileMachines(ctx context.Context, s *scope, infraMachinePool *unstructured.Unstructured) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	mp := s.machinePool

	var infraMachineKind string
	if err := util.UnstructuredUnmarshalField(infraMachinePool, &infraMachineKind, "status", "infrastructureMachineKind"); err != nil {
		if errors.Is(err, util.ErrUnstructuredFieldNotFound) {
			log.V(4).Info("MachinePool Machines not supported, no infraMachineKind found")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve infraMachineKind from infrastructure provider for MachinePool %s", klog.KObj(mp))
	}

	infraMachineSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			clusterv1.MachinePoolNameLabel: format.MustFormatValue(mp.Name),
			clusterv1.ClusterNameLabel:     mp.Spec.ClusterName,
		},
	}

	log.V(4).Info("Reconciling MachinePool Machines", "infrastructureMachineKind", infraMachineKind, "infrastructureMachineSelector", infraMachineSelector)
	var infraMachineList unstructured.UnstructuredList

	// Get the list of infraMachines, which are maintained by the InfraMachinePool controller.
	infraMachineList.SetAPIVersion(infraMachinePool.GetAPIVersion())
	infraMachineList.SetKind(infraMachineKind + "List")
	if err := r.Client.List(ctx, &infraMachineList, client.InNamespace(mp.Namespace), client.MatchingLabels(infraMachineSelector.MatchLabels)); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to list infra machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	// Add watcher for infraMachine, if there isn't one already; this will allow this controller to reconcile
	// immediately changes made by the InfraMachinePool controller.
	sampleInfraMachine := &unstructured.Unstructured{}
	sampleInfraMachine.SetAPIVersion(infraMachinePool.GetAPIVersion())
	sampleInfraMachine.SetKind(infraMachineKind)

	// Add watcher for infraMachine, if there isn't one already.
	if err := r.externalTracker.Watch(log, sampleInfraMachine, handler.EnqueueRequestsFromMapFunc(r.infraMachineToMachinePoolMapper), predicates.ResourceIsChanged(r.Client.Scheme(), *r.externalTracker.PredicateLogger)); err != nil {
		return ctrl.Result{}, err
	}

	// Get the list of machines managed by this controller, and align it with the infra machines managed by
	// the InfraMachinePool controller.
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(mp.Namespace), client.MatchingLabels(infraMachineSelector.MatchLabels)); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createOrUpdateMachines(ctx, s, machineList.Items, infraMachineList.Items); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	res, err := r.reconcileUnhealthyMachinePoolMachines(ctx, s, machineList.Items)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile unhealthy machines for MachinePool %s", klog.KObj(mp))
	}

	return res, nil
}

// createOrUpdateMachines creates a MachinePool Machine for each infraMachine if it doesn't already exist and sets the owner reference and infraRef.
func (r *Reconciler) createOrUpdateMachines(ctx context.Context, s *scope, machines []clusterv1.Machine, infraMachines []unstructured.Unstructured) error {
	log := ctrl.LoggerFrom(ctx)

	// Construct a set of names of infraMachines that already have a Machine.
	infraMachineToMachine := map[string]clusterv1.Machine{}
	for _, machine := range machines {
		infraRef := machine.Spec.InfrastructureRef
		infraMachineToMachine[infraRef.Name] = machine
	}

	createdMachines := []clusterv1.Machine{}
	var errs []error
	for i := range infraMachines {
		infraMachine := &infraMachines[i]

		// Get Spec.ProviderID from the infraMachine.
		var providerID string
		var node *corev1.Node
		if err := util.UnstructuredUnmarshalField(infraMachine, &providerID, "spec", "providerID"); err != nil {
			log.V(4).Info("could not retrieve providerID for infraMachine", "infraMachine", klog.KObj(infraMachine))
		} else {
			// Retrieve the Node for the infraMachine from the nodeRefsMap using the providerID.
			node = s.nodeRefMap[providerID]
		}

		// If infraMachine already has a Machine, update it if needed.
		if existingMachine, ok := infraMachineToMachine[infraMachine.GetName()]; ok {
			log.V(2).Info("Patching existing Machine for infraMachine", infraMachine.GetKind(), klog.KObj(infraMachine), "Machine", klog.KObj(&existingMachine))

			desiredMachine := r.computeDesiredMachine(s.machinePool, infraMachine, &existingMachine, node)
			if err := ssa.Patch(ctx, r.Client, MachinePoolControllerName, desiredMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: &existingMachine}); err != nil {
				log.Error(err, "failed to update Machine", "Machine", klog.KObj(desiredMachine))
				errs = append(errs, errors.Wrapf(err, "failed to update Machine %q", klog.KObj(desiredMachine)))
			}
		} else {
			// Otherwise create a new Machine for the infraMachine.
			log.Info("Creating new Machine for infraMachine", "infraMachine", klog.KObj(infraMachine))
			machine := r.computeDesiredMachine(s.machinePool, infraMachine, nil, node)

			if err := ssa.Patch(ctx, r.Client, MachinePoolControllerName, machine); err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to create new Machine for infraMachine %q in namespace %q", infraMachine.GetName(), infraMachine.GetNamespace()))
				continue
			}

			createdMachines = append(createdMachines, *machine)
		}
	}
	if err := r.waitForMachineCreation(ctx, createdMachines); err != nil {
		errs = append(errs, errors.Wrapf(err, "failed to wait for machines to be created"))
	}
	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}

	return nil
}

// computeDesiredMachine constructs the desired Machine for an infraMachine.
// If the Machine exists, it ensures the Machine always owned by the MachinePool.
func (r *Reconciler) computeDesiredMachine(mp *clusterv1.MachinePool, infraMachine *unstructured.Unstructured, existingMachine *clusterv1.Machine, existingNode *corev1.Node) *clusterv1.Machine {
	infraRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: infraMachine.GroupVersionKind().Group,
		Kind:     infraMachine.GetKind(),
		Name:     infraMachine.GetName(),
	}

	var kubernetesVersion string
	if existingNode != nil && existingNode.Status.NodeInfo.KubeletVersion != "" {
		kubernetesVersion = existingNode.Status.NodeInfo.KubeletVersion
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: infraMachine.GetName(),
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(mp, machinePoolKind)},
			Namespace:       mp.Namespace,
			Labels:          make(map[string]string),
			Annotations:     make(map[string]string),
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: mp.Spec.ClusterName,
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To(""),
			},
			InfrastructureRef: infraRef,
			Version:           kubernetesVersion,
		},
	}

	if existingMachine != nil {
		machine.SetName(existingMachine.Name)
		machine.SetUID(existingMachine.UID)
	}

	for k, v := range mp.Spec.Template.Annotations {
		machine.Annotations[k] = v
	}

	// Set the labels from machinePool.Spec.Template.Labels as labels for the new Machine.
	// Note: We can't just set `machinePool.Spec.Template.Labels` directly and thus "share" the labels
	// map between Machine and machinePool.Spec.Template.Labels. This would mean that adding the
	// MachinePoolNameLabel later on the Machine would also add the labels to machinePool.Spec.Template.Labels
	// and thus modify the labels of the MachinePool.
	for k, v := range mp.Spec.Template.Labels {
		machine.Labels[k] = v
	}

	// Enforce that the MachinePoolNameLabel and ClusterNameLabel are present on the Machine.
	machine.Labels[clusterv1.MachinePoolNameLabel] = format.MustFormatValue(mp.Name)
	machine.Labels[clusterv1.ClusterNameLabel] = mp.Spec.ClusterName

	return machine
}

// infraMachineToMachinePoolMapper is a mapper function that maps an InfraMachine to the MachinePool that owns it.
// This is used to trigger an update of the MachinePool when a InfraMachine is changed.
func (r *Reconciler) infraMachineToMachinePoolMapper(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	if labels.IsMachinePoolOwned(o) {
		machinePool, err := util.GetMachinePoolByLabels(ctx, r.Client, o.GetNamespace(), o.GetLabels())
		if err != nil {
			log.Error(err, "Failed to get MachinePool for InfraMachine", o.GetObjectKind().GroupVersionKind().Kind, klog.KObj(o), "labels", o.GetLabels())
			return nil
		}
		if machinePool != nil {
			return []ctrl.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: machinePool.Namespace,
						Name:      machinePool.Name,
					},
				},
			}
		}
	}

	return nil
}

func (r *Reconciler) waitForMachineCreation(ctx context.Context, machineList []clusterv1.Machine) error {
	_ = ctrl.LoggerFrom(ctx)

	// waitForCacheUpdateTimeout is the amount of time allowed to wait for desired state.
	const waitForCacheUpdateTimeout = 10 * time.Second

	// waitForCacheUpdateInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	const waitForCacheUpdateInterval = 100 * time.Millisecond

	for i := range machineList {
		machine := machineList[i]
		pollErr := wait.PollUntilContextTimeout(ctx, waitForCacheUpdateInterval, waitForCacheUpdateTimeout, true, func(ctx context.Context) (bool, error) {
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}
			if err := r.Client.Get(ctx, key, &clusterv1.Machine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			return errors.Wrapf(pollErr, "failed waiting for machine object %v to be created", klog.KObj(&machine))
		}
	}

	return nil
}

func (r *Reconciler) getNodeRefMap(ctx context.Context, c client.Client) (map[string]*corev1.Node, error) {
	log := ctrl.LoggerFrom(ctx)
	nodeRefsMap := make(map[string]*corev1.Node)
	nodeList := corev1.NodeList{}
	// Note: We don't use pagination as this is a cached client and a cached client doesn't support pagination.
	if err := c.List(ctx, &nodeList); err != nil {
		return nil, err
	}

	for _, node := range nodeList.Items {
		if node.Spec.ProviderID == "" {
			log.V(2).Info("No ProviderID detected, skipping", "providerID", node.Spec.ProviderID)
			continue
		}

		nodeRefsMap[node.Spec.ProviderID] = &node
	}

	return nodeRefsMap, nil
}

func patchMachineConditions(ctx context.Context, c client.Client, machines []*clusterv1.Machine, condition metav1.Condition, v1beta1condition *clusterv1.Condition) error {
	var errs []error
	for _, m := range machines {
		patchHelper, err := patch.NewHelper(m, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if v1beta1condition != nil {
			v1beta1conditions.Set(m, v1beta1condition)
		}
		conditions.Set(m, condition)

		if err := patchHelper.Patch(ctx, m,
			patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			}}, patch.WithOwnedConditions{Conditions: []string{
				clusterv1.MachineOwnerRemediatedCondition,
			}}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Wrapf(kerrors.NewAggregate(errs), "failed to patch Machines")
	}

	return nil
}

func (r *Reconciler) reconcileUnhealthyMachinePoolMachines(ctx context.Context, s *scope, machines []clusterv1.Machine) (ctrl.Result, error) {
	if len(machines) == 0 {
		return ctrl.Result{}, nil
	}

	log := ctrl.LoggerFrom(ctx)
	mp := s.machinePool

	machinesWithHealthCheck := slices.DeleteFunc(slices.Clone(machines), func(machine clusterv1.Machine) bool {
		return !conditions.Has(&machine, clusterv1.MachineHealthCheckSucceededCondition)
	})
	if len(machinesWithHealthCheck) == 0 {
		// This means there is no MachineHealthCheck selecting any machines
		// of this machine pool. In this case, do not requeue so often,
		// but still check regularly in case a MachineHealthCheck became
		// deployed or activated. This long interval shouldn't be a problem
		// at cluster creation, since newly-created nodes should anyway
		// trigger MachinePool reconciliation as the infrastructure provider
		// creates the InfraMachines.
		log.V(4).Info("Skipping reconciliation of unhealthy MachinePool machines because there are no health-checked machines")
		return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
	}

	unhealthyMachines := slices.DeleteFunc(slices.Clone(machines), func(machine clusterv1.Machine) bool {
		return !collections.IsUnhealthyAndOwnerRemediated(&machine)
	})
	log.V(4).Info("Reconciling unhealthy MachinePool machines", "unhealthyMachines", len(unhealthyMachines))

	// Calculate how many in flight machines we should remediate.
	// By default, we allow all machines to be remediated at the same time.
	maxInFlight := len(unhealthyMachines)
	if mp.Spec.Remediation.MaxInFlight != nil {
		var err error
		replicas := int(ptr.Deref(mp.Spec.Replicas, 1))
		maxInFlight, err = intstr.GetScaledValueFromIntOrPercent(mp.Spec.Remediation.MaxInFlight, replicas, true)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to calculate maxInFlight to remediate machines: %v", err)
		}
		log = log.WithValues("maxInFlight", maxInFlight, "replicas", replicas)
	}

	machinesToRemediate := make([]*clusterv1.Machine, 0, len(unhealthyMachines))
	inFlight := 0
	for _, m := range unhealthyMachines {
		if !m.DeletionTimestamp.IsZero() {
			if conditions.IsTrue(&m, clusterv1.MachineOwnerRemediatedCondition) {
				// Machine has been remediated by this controller and still in flight.
				inFlight++
			}
			continue
		}
		if conditions.IsFalse(&m, clusterv1.MachineOwnerRemediatedCondition) {
			machinesToRemediate = append(machinesToRemediate, &m)
		}
	}
	log = log.WithValues("inFlight", inFlight)

	if len(machinesToRemediate) == 0 {
		log.V(5).Info("No MachinePool machines to remediate, requeueing")

		// There's a MachineHealthCheck monitoring the machines, but currently
		// no action to be taken. A machine could require remediation at any
		// time, so use a short interval until next reconciliation.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if inFlight >= maxInFlight {
		log.V(3).Info("Remediation strategy is set, and maximum in flight has been reached", "machinesToBeRemediated", len(machinesToRemediate))

		if err := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolMachineRemediationDeferredReason,
			Message: fmt.Sprintf("Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is %s)", mp.Spec.Remediation.MaxInFlight),
		}, nil); err != nil {
			return ctrl.Result{}, err
		}

		// Once the infrastructure provider updates `providerIDList`, MachinePool reconciliation
		// will be triggered again, leading to the next check whether already-remediating (= deleting)
		// machines are gone so that more machines could be remediated
		return ctrl.Result{}, nil
	}

	remediation.SortMachinesToRemediate(machinesToRemediate)

	haveMoreMachinesToRemediate := false
	if len(machinesToRemediate) > (maxInFlight - inFlight) {
		haveMoreMachinesToRemediate = true
		log.V(5).Info("Remediation strategy is set, limiting in flight operations", "machinesToBeRemediated", len(machinesToRemediate))
		machinesToRemediate = machinesToRemediate[:(maxInFlight - inFlight)]
	}

	// Run preflight checks.
	preflightCheckErrMessages, err := r.runPreflightChecks(ctx, s.cluster, mp, "MachinePool machine remediation")
	if err != nil || len(preflightCheckErrMessages) > 0 {
		if err != nil {
			// If err is not nil use that as the preflightCheckErrMessage
			preflightCheckErrMessages = append(preflightCheckErrMessages, err.Error())
		}

		listMessages := make([]string, len(preflightCheckErrMessages))
		for i, msg := range preflightCheckErrMessages {
			listMessages[i] = fmt.Sprintf("* %s", msg)
		}

		// PreflightChecks did not pass. Update the MachineOwnerRemediated condition on the unhealthy Machines with
		// WaitingForRemediationReason reason.
		if patchErr := patchMachineConditions(ctx, r.Client, machinesToRemediate, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolMachineRemediationDeferredReason,
			Message: strings.Join(listMessages, "\n"),
		}, &clusterv1.Condition{
			Type:     clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			Status:   corev1.ConditionFalse,
			Reason:   clusterv1.WaitingForRemediationV1Beta1Reason,
			Severity: clusterv1.ConditionSeverityWarning,
			Message:  strings.Join(preflightCheckErrMessages, "; "),
		}); patchErr != nil {
			return ctrl.Result{}, kerrors.NewAggregate([]error{err, patchErr})
		}

		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	// PreflightChecks passed, so it is safe to remediate unhealthy machines by deleting them.

	// Remediate unhealthy machines by deleting them
	var errs []error
	for _, m := range machinesToRemediate {
		log.Info("Deleting unhealthy Machine", "Machine", klog.KObj(m))
		patch := client.MergeFrom(m.DeepCopy())
		if err := r.Client.Delete(ctx, m); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, errors.Wrapf(err, "failed to delete Machine %s", klog.KObj(m)))
			continue
		}
		v1beta1conditions.MarkTrue(m, clusterv1.MachineOwnerRemediatedCondition)
		if err := r.Client.Status().Patch(ctx, m, patch); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrapf(err, "failed to update status of Machine %s", klog.KObj(m)))
		}
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errors.Wrapf(kerrors.NewAggregate(errs), "failed to delete unhealthy Machines")
	}

	if haveMoreMachinesToRemediate {
		// More machines need remediation, so reconcile again sooner
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
