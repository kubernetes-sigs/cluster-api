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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
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
	if mp.Status.InfrastructureReady && mp.Spec.Replicas != nil && *mp.Spec.Replicas == mp.Status.ReadyReplicas {
		mp.Status.SetTypedPhase(expv1.MachinePoolPhaseRunning)
	}

	// Set the appropriate phase in response to the MachinePool replica count being greater than the observed infrastructure replicas.
	if mp.Status.InfrastructureReady && mp.Spec.Replicas != nil && *mp.Spec.Replicas > mp.Status.ReadyReplicas {
		// If we are being managed by an external autoscaler and can't predict scaling direction, set to "Scaling".
		if annotations.ReplicasManagedByExternalAutoscaler(mp) {
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScaling)
		} else {
			// Set the phase to "ScalingUp" if we are actively scaling the infrastructure out.
			mp.Status.SetTypedPhase(expv1.MachinePoolPhaseScalingUp)
		}
	}

	// Set the appropriate phase in response to the MachinePool replica count being less than the observed infrastructure replicas.
	if mp.Status.InfrastructureReady && mp.Spec.Replicas != nil && *mp.Spec.Replicas < mp.Status.ReadyReplicas {
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

	// Ensure we add a watch to the external object, if there isn't one already.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &expv1.MachinePool{})); err != nil {
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
		m.Status.FailureMessage = ptr.To(
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
			log.Info("Waiting for bootstrap provider to generate data secret and report status.ready", bootstrapConfig.GetKind(), klog.KObj(bootstrapConfig))
			m.Status.BootstrapReady = ready
			return ctrl.Result{}, nil
		}

		// Get and set the name of the secret containing the bootstrap data.
		secretName, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "dataSecretName")
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		} else if secretName == "" {
			return ctrl.Result{}, errors.Errorf("retrieved empty dataSecretName from bootstrap provider for MachinePool %q in namespace %q", m.Name, m.Namespace)
		}

		m.Spec.Template.Spec.Bootstrap.DataSecretName = ptr.To(secretName)
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
				mp.Status.FailureReason = ptr.To(capierrors.InvalidConfigurationMachinePoolError)
				mp.Status.FailureMessage = ptr.To(fmt.Sprintf("MachinePool infrastructure resource %v with name %q has been deleted after being ready",
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

	if err := r.reconcileMachines(ctx, mp, infraConfig); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile Machines for MachinePool %s", klog.KObj(mp))
	}

	if !mp.Status.InfrastructureReady {
		log.Info("Infrastructure provider is not yet ready", infraConfig.GetKind(), klog.KObj(infraConfig))
		return ctrl.Result{}, nil
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
		log.Info("Retrieved empty spec.providerIDList from infrastructure provider but status.replicas is not zero.", "replicas", mp.Status.Replicas)
		return ctrl.Result{}, nil
	}

	if !reflect.DeepEqual(mp.Spec.ProviderIDList, providerIDList) {
		mp.Spec.ProviderIDList = providerIDList
		mp.Status.ReadyReplicas = 0
		mp.Status.AvailableReplicas = 0
		mp.Status.UnavailableReplicas = mp.Status.Replicas
	}

	return ctrl.Result{}, nil
}

// reconcileMachines reconciles Machines associated with a MachinePool.
//
// Note: In the case of MachinePools the machines are created in order to surface in CAPI what exists in the
// infrastructure while instead on MachineDeployments, machines are created in CAPI first and then the
// infrastructure is created accordingly.
// Note: When supported by the cloud provider implementation of the MachinePool, machines will provide a means to interact
// with the corresponding infrastructure (e.g. delete a specific machine in case MachineHealthCheck detects it is unhealthy).
func (r *MachinePoolReconciler) reconcileMachines(ctx context.Context, mp *expv1.MachinePool, infraMachinePool *unstructured.Unstructured) error {
	log := ctrl.LoggerFrom(ctx)

	var infraMachineKind string
	if err := util.UnstructuredUnmarshalField(infraMachinePool, &infraMachineKind, "status", "infrastructureMachineKind"); err != nil {
		if errors.Is(err, util.ErrUnstructuredFieldNotFound) {
			log.V(4).Info("MachinePool Machines not supported, no infraMachineKind found")
			return nil
		}

		return errors.Wrapf(err, "failed to retrieve infraMachineKind from infrastructure provider for MachinePool %s", klog.KObj(mp))
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
		return errors.Wrapf(err, "failed to list infra machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	// Add watcher for infraMachine, if there isn't one already; this will allow this controller to reconcile
	// immediately changes made by the InfraMachinePool controller.
	sampleInfraMachine := &unstructured.Unstructured{}
	sampleInfraMachine.SetAPIVersion(infraMachinePool.GetAPIVersion())
	sampleInfraMachine.SetKind(infraMachineKind)

	// Add watcher for infraMachine, if there isn't one already.
	if err := r.externalTracker.Watch(log, sampleInfraMachine, handler.EnqueueRequestsFromMapFunc(r.infraMachineToMachinePoolMapper)); err != nil {
		return err
	}

	// Get the list of machines managed by this controller, and align it with the infra machines managed by
	// the InfraMachinePool controller.
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(mp.Namespace), client.MatchingLabels(infraMachineSelector.MatchLabels)); err != nil {
		return err
	}

	if err := r.createOrUpdateMachines(ctx, mp, machineList.Items, infraMachineList.Items); err != nil {
		return errors.Wrapf(err, "failed to create machines for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	return nil
}

// createOrUpdateMachines creates a MachinePool Machine for each infraMachine if it doesn't already exist and sets the owner reference and infraRef.
func (r *MachinePoolReconciler) createOrUpdateMachines(ctx context.Context, mp *expv1.MachinePool, machines []clusterv1.Machine, infraMachines []unstructured.Unstructured) error {
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
		// If infraMachine already has a Machine, update it if needed.
		if existingMachine, ok := infraMachineToMachine[infraMachine.GetName()]; ok {
			log.V(2).Info("Patching existing Machine for infraMachine", "infraMachine", klog.KObj(infraMachine), "machine", klog.KObj(&existingMachine))

			desiredMachine := computeDesiredMachine(mp, infraMachine, &existingMachine)
			if err := ssa.Patch(ctx, r.Client, MachinePoolControllerName, desiredMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: &existingMachine}); err != nil {
				log.Error(err, "failed to update Machine", "Machine", klog.KObj(desiredMachine))
				errs = append(errs, errors.Wrapf(err, "failed to update Machine %q", klog.KObj(desiredMachine)))
			}
		} else {
			// Otherwise create a new Machine for the infraMachine.
			log.Info("Creating new Machine for infraMachine", "infraMachine", klog.KObj(infraMachine))
			machine := computeDesiredMachine(mp, infraMachine, nil)

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
func computeDesiredMachine(mp *expv1.MachinePool, infraMachine *unstructured.Unstructured, existingMachine *clusterv1.Machine) *clusterv1.Machine {
	infraRef := corev1.ObjectReference{
		APIVersion: infraMachine.GetAPIVersion(),
		Kind:       infraMachine.GetKind(),
		Name:       infraMachine.GetName(),
		Namespace:  infraMachine.GetNamespace(),
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
			ClusterName:       mp.Spec.ClusterName,
			InfrastructureRef: infraRef,
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
func (r *MachinePoolReconciler) infraMachineToMachinePoolMapper(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	if labels.IsMachinePoolOwned(o) {
		machinePool, err := utilexp.GetMachinePoolByLabels(ctx, r.Client, o.GetNamespace(), o.GetLabels())
		if err != nil {
			log.Error(err, "failed to get MachinePool for InfraMachine", "infraMachine", klog.KObj(o), "labels", o.GetLabels())
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

func (r *MachinePoolReconciler) waitForMachineCreation(ctx context.Context, machineList []clusterv1.Machine) error {
	_ = ctrl.LoggerFrom(ctx)

	// waitForCacheUpdateTimeout is the amount of time allowed to wait for desired state.
	const waitForCacheUpdateTimeout = 10 * time.Second

	// waitForCacheUpdateInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	const waitForCacheUpdateInterval = 100 * time.Millisecond

	for i := 0; i < len(machineList); i++ {
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
