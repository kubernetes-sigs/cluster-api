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
	"reflect"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	capilabels "sigs.k8s.io/cluster-api/internal/labels"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/patch"
)

var (
	externalReadyWait = 30 * time.Second
)

func (r *MachinePoolReconciler) reconcilePhase(mp *expv1.MachinePool) {
	// Set the phase to "pending" if nil.
	if mp.Status.Phase == "" {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if mp.Status.BootstrapReady && !mp.Status.InfrastructureReady {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if len(mp.Status.NodeRefs) != 0 {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseProvisioned)
	}

	// Set the phase to "running" if the number of ready replicas is equal to desired replicas.
	if mp.Status.InfrastructureReady && *mp.Spec.Replicas == mp.Status.ReadyReplicas {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseRunning)
	}

	// Set the appropriate phase in response to the MachinePool replica count being greater than the observed infrastructure replicas.
	if mp.Status.InfrastructureReady && *mp.Spec.Replicas > mp.Status.ReadyReplicas {
		// If we are being managed by an external autoscaler and can't predict scaling direction, set to "Scaling".
		if annotations.ReplicasManagedByExternalAutoscaler(mp) {
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScaling)
		} else {
			// Set the phase to "ScalingUp" if we are actively scaling the infrastructure out.
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScalingUp)
		}
	}

	// Set the appropriate phase in response to the MachinePool replica count being less than the observed infrastructure replicas.
	if mp.Status.InfrastructureReady && *mp.Spec.Replicas < mp.Status.ReadyReplicas {
		// If we are being managed by an external autoscaler and can't predict scaling direction, set to "Scaling".
		if annotations.ReplicasManagedByExternalAutoscaler(mp) {
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScaling)
		} else {
			// Set the phase to "ScalingDown" if we are actively scaling the infrastructure in.
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScalingDown)
		}
	}

	// Set the phase to "failed" if any of Status.FailureReason or Status.FailureMessage is not-nil.
	if mp.Status.FailureReason != nil || mp.Status.FailureMessage != nil {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !mp.DeletionTimestamp.IsZero() {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a MachinePool.
func (r *MachinePoolReconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, m *expv1.MachinePool, ref *corev1.ObjectReference) (external.ReconcileOutput, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return external.ReconcileOutput{}, err
	}

	obj, err := external.Get(ctx, r.Client, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return external.ReconcileOutput{}, errors.Wrapf(err, "could not find %v %q for MachinePool %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		return external.ReconcileOutput{}, err
	}

	// if external ref is paused, return error.
	if annotations.IsPaused(cluster, obj) {
		log.V(3).Info("External object referenced is paused")
		return external.ReconcileOutput{Paused: true}, nil
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

	// Add watcher for external object, if there isn't one already.
	_, loaded := r.externalWatchers.LoadOrStore(obj.GroupVersionKind().String(), struct{}{})
	if !loaded && r.controller != nil {
		log.Info("Adding watcher on external object", "groupVersionKind", obj.GroupVersionKind())
		err := r.controller.Watch(
			source.Kind(r.cache, obj),
			handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &expv1.MachinePool{}),
		)
		if err != nil {
			r.externalWatchers.Delete(obj.GroupVersionKind().String())
			return external.ReconcileOutput{}, errors.Wrapf(err, "failed to add watcher on external object %q", obj.GroupVersionKind())
		}
	}

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return external.ReconcileOutput{}, err
	}
	if failureReason != "" {
		machineStatusFailure := capierrors.MachinePoolStatusFailure(failureReason)
		m.Status.FailureReason = &machineStatusFailure
	}
	if failureMessage != "" {
		m.Status.FailureMessage = pointer.String(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return external.ReconcileOutput{Result: obj}, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a MachinePool.
func (r *MachinePoolReconciler) reconcileBootstrap(ctx context.Context, cluster *clusterv1.Cluster, m *expv1.MachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Call generic external reconciler if we have an external reference.
	var bootstrapConfig *unstructured.Unstructured
	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		bootstrapReconcileResult, err := r.reconcileExternal(ctx, cluster, m, m.Spec.Template.Spec.Bootstrap.ConfigRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		// if the external object is paused, return without any further processing
		if bootstrapReconcileResult.Paused {
			return ctrl.Result{}, nil
		}
		bootstrapConfig = bootstrapReconcileResult.Result

		// If the bootstrap config is being deleted, return early.
		if !bootstrapConfig.GetDeletionTimestamp().IsZero() {
			return ctrl.Result{}, nil
		}

		// Determine if the bootstrap provider is ready.
		ready, err := external.IsReady(bootstrapConfig)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Report a summary of current status of the bootstrap object defined for this machine pool.
		conditions.SetMirror(m, clusterv1.BootstrapReadyCondition,
			conditions.UnstructuredGetter(bootstrapConfig),
			conditions.WithFallbackValue(ready, clusterv1.WaitingForDataSecretFallbackReason, clusterv1.ConditionSeverityInfo, ""),
		)

		if !ready {
			log.V(2).Info("Bootstrap provider is not ready, requeuing")
			m.Status.BootstrapReady = ready
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}

		// Get and set the name of the secret containing the bootstrap data.
		secretName, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "dataSecretName")
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		} else if secretName == "" {
			return ctrl.Result{}, errors.Errorf("retrieved empty dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		}

		m.Spec.Template.Spec.Bootstrap.DataSecretName = pointer.String(secretName)
		m.Status.BootstrapReady = true
		return ctrl.Result{}, nil
	}

	// If dataSecretName is set without a ConfigRef, this means the user brought their own bootstrap data.
	if m.Spec.Template.Spec.Bootstrap.DataSecretName != nil {
		m.Status.BootstrapReady = true
		conditions.MarkTrue(m, clusterv1.BootstrapReadyCondition)
		return ctrl.Result{}, nil
	}

	// This should never happen because the MachinePool webhook would not allow neither ConfigRef nor DataSecretName to be set.
	return ctrl.Result{}, errors.Errorf("neither .spec.bootstrap.configRef nor .spec.bootstrap.dataSecretName are set for MachinePool %q in namespace %q", m.Name, m.Namespace)
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a MachinePool.
func (r *MachinePoolReconciler) reconcileInfrastructure(ctx context.Context, cluster *clusterv1.Cluster, mp *expv1.MachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Call generic external reconciler.
	infraReconcileResult, err := r.reconcileExternal(ctx, cluster, mp, &mp.Spec.Template.Spec.InfrastructureRef)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			log.Error(err, "infrastructure reference could not be found")
			if mp.Status.InfrastructureReady {
				// Infra object went missing after the machine pool was up and running
				log.Error(err, "infrastructure reference has been deleted after being ready, setting failure state")
				mp.Status.FailureReason = capierrors.MachinePoolStatusErrorPtr(capierrors.InvalidConfigurationMachinePoolError)
				mp.Status.FailureMessage = pointer.String(fmt.Sprintf("MachinePool infrastructure resource %v with name %q has been deleted after being ready",
					mp.Spec.Template.Spec.InfrastructureRef.GroupVersionKind(), mp.Spec.Template.Spec.InfrastructureRef.Name))
			}
			conditions.MarkFalse(mp, clusterv1.InfrastructureReadyCondition, clusterv1.IncorrectExternalRefReason, clusterv1.ConditionSeverityError, fmt.Sprintf("could not find infra reference of kind %s with name %s", mp.Spec.Template.Spec.InfrastructureRef.Kind, mp.Spec.Template.Spec.InfrastructureRef.Name))
		}
		return ctrl.Result{}, err
	}
	// if the external object is paused, return without any further processing
	if infraReconcileResult.Paused {
		return ctrl.Result{}, nil
	}
	infraConfig := infraReconcileResult.Result

	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	mp.Status.InfrastructureReady = ready

	// Report a summary of current status of the infrastructure object defined for this machine pool.
	conditions.SetMirror(mp, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(infraConfig),
		conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
	)

	if err := r.reconcileMachinePoolMachines(ctx, mp, infraConfig); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile MachinePool machines for MachinePool `%s`", mp.Name)
	}

	if !mp.Status.InfrastructureReady {
		log.Info("Infrastructure provider is not ready, requeuing")
		return ctrl.Result{RequeueAfter: externalReadyWait}, nil
	}

	var providerIDList []string
	// Get Spec.ProviderIDList from the infrastructure provider.
	if err := util.UnstructuredUnmarshalField(infraConfig, &providerIDList, "spec", "providerIDList"); err != nil && !errors.Is(err, util.ErrUnstructuredFieldNotFound) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve data from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	// Get and set Status.Replicas from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &mp.Status.Replicas, "status", "replicas")
	if err != nil {
		if err != util.ErrUnstructuredFieldNotFound {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve replicas from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
		}
	}

	if len(providerIDList) == 0 && mp.Status.Replicas != 0 {
		log.Info("Retrieved empty Spec.ProviderIDList from infrastructure provider but Status.Replicas is not zero.", "replicas", mp.Status.Replicas)
		return ctrl.Result{RequeueAfter: externalReadyWait}, nil
	}

	if !reflect.DeepEqual(mp.Spec.ProviderIDList, providerIDList) {
		mp.Spec.ProviderIDList = providerIDList
		mp.Status.ReadyReplicas = 0
		mp.Status.AvailableReplicas = 0
		mp.Status.UnavailableReplicas = mp.Status.Replicas
	}

	return ctrl.Result{}, nil
}

// reconcileMachinePoolMachines reconciles MachinePool Machines associated with a MachinePool.
func (r *MachinePoolReconciler) reconcileMachinePoolMachines(ctx context.Context, mp *expv1.MachinePool, infraMachinePool *unstructured.Unstructured) error {
	log := ctrl.LoggerFrom(ctx)

	var noKind bool
	var noSelector bool

	var infraMachineKind string
	if err := util.UnstructuredUnmarshalField(infraMachinePool, &infraMachineKind, "status", "infrastructureMachineKind"); err != nil {
		if errors.Is(err, util.ErrUnstructuredFieldNotFound) {
			noKind = true
		} else {
			return errors.Wrapf(err, "failed to retrieve infraMachineKind from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
		}
	}

	var infraMachineSelector metav1.LabelSelector
	if err := util.UnstructuredUnmarshalField(infraMachinePool, &infraMachineSelector, "status", "infrastructureMachineSelector"); err != nil {
		if errors.Is(err, util.ErrUnstructuredFieldNotFound) {
			noSelector = true
		} else {
			return errors.Wrapf(err, "failed to retrieve infraMachineSelector from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
		}
	}

	// If one missing but not both, return error
	if noKind != noSelector {
		return errors.Errorf("only one of infraMachineKind or infraMachineSelector found, both must be present")
	}

	// If proper MachinePool Machines aren't supported by the InfraMachinePool, just create Machines to match the replica count.
	if noKind && noSelector {
		log.Info("No infraMachineKind or infraMachineSelector found, reconciling fallback machines")
		return r.reconcileFallbackMachines(ctx, mp)
	}

	log.Info("Reconciling MachinePool Machines", "infrastructureMachineKind", infraMachineKind, "infrastructureMachineSelector", infraMachineSelector)
	var infraMachineList unstructured.UnstructuredList

	infraMachineList.SetAPIVersion(infraMachinePool.GetAPIVersion())
	infraMachineList.SetKind(infraMachineKind)

	if err := r.Client.List(ctx, &infraMachineList, client.InNamespace(mp.Namespace), client.MatchingLabels(infraMachineSelector.MatchLabels)); err != nil {
		return errors.Wrapf(err, "failed to list infra machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	for i := range infraMachineList.Items {
		infraMachine := &infraMachineList.Items[i]
		// Add watcher for external object, if there isn't one already.
		_, loaded := r.externalWatchers.LoadOrStore(infraMachine.GroupVersionKind().String(), struct{}{})
		if !loaded && r.controller != nil {
			log.Info("Adding watcher on external object", "groupVersionKind", infraMachine.GroupVersionKind())
			err := r.controller.Watch(
				&source.Kind{Type: infraMachine},
				// &handler.EnqueueRequestForOwner{OwnerType: &expv1.MachinePool{}},
				handler.EnqueueRequestsFromMapFunc(infraMachineToMachinePoolMapper),
			)
			if err != nil {
				r.externalWatchers.Delete(infraMachine.GroupVersionKind().String())
				return errors.Wrapf(err, "failed to add watcher on external object %q", infraMachine.GroupVersionKind())
			}
		}
	}

	machineList := &clusterv1.MachineList{}
	labels := map[string]string{
		clusterv1.MachinePoolNameLabel: mp.Name,
		clusterv1.ClusterNameLabel:     mp.Spec.ClusterName,
	}

	if err := r.Client.List(ctx, machineList, client.InNamespace(mp.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	if err := r.deleteDanglingMachines(ctx, mp, machineList.Items); err != nil {
		return errors.Wrapf(err, "failed to clean up orphan machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	if err := r.createMachinesIfNotExists(ctx, mp, machineList.Items, infraMachineList.Items); err != nil {
		return errors.Wrapf(err, "failed to create machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	if err := r.setInfraMachineOwnerRefs(ctx, mp, infraMachineList.Items); err != nil {
		return errors.Wrapf(err, "failed to create machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	return nil
}

// deleteDanglingMachines deletes all MachinePool Machines with an infraRef pointing to an infraMachine that no longer exists.
// This allows us to clean up Machines when the MachinePool is scaled down.
func (r *MachinePoolReconciler) deleteDanglingMachines(ctx context.Context, mp *expv1.MachinePool, machines []clusterv1.Machine) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Deleting orphaned machines", "machinePool", mp.Name, "namespace", mp.Namespace)

	for i := range machines {
		machine := &machines[i]
		infraRef := machine.Spec.InfrastructureRef

		// Check this here since external.Get() will complain if the infraRef is empty, i.e. no object kind or apiVersion.
		if (infraRef == corev1.ObjectReference{}) {
			// Until the InfraMachinePool populates its selector kind and selector labels, the MachinePool controller can't tell that it supports MachinePool Machines
			// and will create fallback machines to begin with. When the selector fields gets populated, we want to clean up those fallback machines.
			log.V(2).Info("Machine has an empty infraRef, this a fallback machine, will delete", "machine", machine.Name, "namespace", machine.Namespace)
			if err := r.Client.Delete(ctx, machine); err != nil {
				return err
			}

			continue
		}

		_, err := external.Get(ctx, r.Client, &infraRef, mp.Namespace)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "unexpected error while trying to get infra machine, will delete anyway", "machine", machine.Name, "namespace", machine.Namespace)
			}

			log.V(2).Info("Deleting orphaned machine", "machine", machine.Name, "namespace", machine.Namespace)
			if err := r.Client.Delete(ctx, machine); err != nil {
				return err
			}
		} else {
			log.V(2).Info("Machine is not orphaned, nothing to do", "machine", machine.Name, "namespace", machine.Namespace)
		}
	}

	return nil
}

// createMachinesIfNotExists creates a MachinePool Machine for each infraMachine if it doesn't already exist and sets the owner reference and infraRef.
func (r *MachinePoolReconciler) createMachinesIfNotExists(ctx context.Context, mp *expv1.MachinePool, machines []clusterv1.Machine, infraMachines []unstructured.Unstructured) error {
	infraRefNames := sets.Set[string]{}
	for _, machine := range machines {
		infraRef := machine.Spec.InfrastructureRef
		infraRefNames.Insert(infraRef.Name)
	}

	for i := range infraMachines {
		infraMachine := &infraMachines[i]
		if infraRefNames.Has(infraMachine.GetName()) {
			continue
		}

		machine := getNewMachine(mp, infraMachine, false)
		if err := r.Client.Create(ctx, machine); err != nil {
			return errors.Wrapf(err, "failed to create new Machine for infraMachine %q in namespace %q", infraMachine.GetName(), infraMachine.GetNamespace())
		}
	}

	return nil
}

// setInfraMachineOwnerRefs creates a MachinePool Machine for each infraMachine if it doesn't already exist and sets the owner reference and infraRef.
func (r *MachinePoolReconciler) setInfraMachineOwnerRefs(ctx context.Context, mp *expv1.MachinePool, infraMachines []unstructured.Unstructured) error {
	machineList := &clusterv1.MachineList{}
	labels := map[string]string{
		clusterv1.MachinePoolNameLabel: mp.Name,
		clusterv1.ClusterNameLabel:     mp.Spec.ClusterName,
	}

	if err := r.Client.List(ctx, machineList, client.InNamespace(mp.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	infraMachineNameToMachine := make(map[string]clusterv1.Machine)
	for _, machine := range machineList.Items {
		infraRef := machine.Spec.InfrastructureRef
		infraMachineNameToMachine[infraRef.Name] = machine
	}

	for i := range infraMachines {
		infraMachine := &infraMachines[i]
		ownerRefs := infraMachine.GetOwnerReferences()
		hasOwnerMachine := false
		for _, ownerRef := range ownerRefs {
			if ownerRef.Kind == "Machine" && ownerRef.APIVersion == clusterv1.GroupVersion.String() {
				hasOwnerMachine = true
				break
			}
		}

		if !hasOwnerMachine {
			machine, ok := infraMachineNameToMachine[infraMachine.GetName()]
			if !ok {
				return errors.Errorf("failed to patch ownerRef for infraMachine %q because no Machine has an infraRef pointing to it", infraMachine.GetName())
			}
			// Set the owner reference on the infraMachine to the Machine since the infraMachine is created and owned by the infraMachinePool.
			infraMachine.SetOwnerReferences([]metav1.OwnerReference{
				*metav1.NewControllerRef(&machine, machine.GroupVersionKind()),
			})

			patchHelper, err := patch.NewHelper(infraMachine, r.Client)
			if err != nil {
				return errors.Wrapf(err, "failed to create patch helper for infraMachine %q in namespace %q", infraMachine.GetName(), infraMachine.GetNamespace())
			}

			if err := patchHelper.Patch(ctx, infraMachine); err != nil {
				return errors.Wrapf(err, "failed to patch infraMachine %q in namespace %q", infraMachine.GetName(), infraMachine.GetNamespace())
			}
		}
	}

	return nil
}

// reconcileFallbackMachines creates Machines for InfraMachinePools that do not support MachinePool Machines.
// These Machines will contain only basic information and exist to make a more consistent user experience across all MachinePools and MachineDeployments and do not have an infraRef.
func (r *MachinePoolReconciler) reconcileFallbackMachines(ctx context.Context, mp *expv1.MachinePool) error {
	machineList := &clusterv1.MachineList{}
	labels := map[string]string{
		clusterv1.MachinePoolNameLabel: mp.Name,
		clusterv1.ClusterNameLabel:     mp.Spec.ClusterName,
	}

	if err := r.Client.List(ctx, machineList, client.InNamespace(mp.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	numMachines := len(machineList.Items)

	if mp.Spec.Replicas == nil {
		return errors.New("MachinePool.Spec.Replicas is nil, this is unexpected")
	}

	// If we have more machines than the desired number of replicas, delete machines until we have the correct number.
	for numMachines > int(*mp.Spec.Replicas) {
		machine := &machineList.Items[numMachines-1]
		if err := r.Client.Delete(ctx, machine); err != nil {
			return errors.Wrapf(err, "failed to delete Machine %q in namespace %q", machineList.Items[numMachines-1].Name, machineList.Items[numMachines-1].Namespace)
		}
		numMachines--
	}

	// If we have less machines than the desired number of replicas, create machines until we have the correct number.
	for numMachines < int(*mp.Spec.Replicas) {
		machine := getNewMachine(mp, nil, true)
		if err := r.Client.Create(ctx, machine); err != nil {
			return errors.Wrap(err, "failed to create new Machine")
		}
		numMachines++
	}

	return nil
}

// getNewMachine creates a new Machine object. The name of the newly created resource is going
// to be created by the API server, we set the generateName field.
func getNewMachine(mp *expv1.MachinePool, infraMachine *unstructured.Unstructured, isFallback bool) *clusterv1.Machine {
	infraRef := corev1.ObjectReference{}
	if infraMachine != nil {
		infraRef.APIVersion = infraMachine.GetAPIVersion()
		infraRef.Kind = infraMachine.GetKind()
		infraRef.Name = infraMachine.GetName()
		infraRef.Namespace = infraMachine.GetNamespace()
	}

	annotations := mp.Spec.Template.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", mp.Name),
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(mp, mp.GroupVersionKind())},
			Namespace:       mp.Namespace,
			Labels:          make(map[string]string),
			Annotations:     annotations,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       mp.Spec.ClusterName,
			InfrastructureRef: infraRef,
		},
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
	machine.Labels[clusterv1.MachinePoolNameLabel] = capilabels.MustFormatValue(mp.Name)
	machine.Labels[clusterv1.ClusterNameLabel] = mp.Spec.ClusterName

	if isFallback {
		labels.SetObjectLabel(machine, clusterv1.FallbackMachineLabel)
	}

	return machine
}
