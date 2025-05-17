/*
Copyright 2024 The Kubernetes Authors.

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

package inmemory

import (
	"context"
	"crypto/rsa"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

// MachineBackendReconciler reconciles a InMemoryMachine object.
type MachineBackendReconciler struct {
	client.Client
	InMemoryManager inmemoryruntime.Manager
	APIServerMux    *inmemoryserver.WorkloadClustersMux
}

// ReconcileNormal handle in memory backend for DevMachine not yet deleted.
func (r *MachineBackendReconciler) ReconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.DevCluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	if inMemoryMachine.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevMachines without an InMemory backend")
	}
	if inMemoryCluster.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevCluster without an InMemory backend")
	}
	log := ctrl.LoggerFrom(ctx)

	setOtherWaitingConditions := func() {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryNodeWaitingForVMProvisionedV1Beta2Reason,
		})

		if !util.IsControlPlaneMachine(machine) {
			return
		}

		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryEtcdWaitingForVMProvisionedV1Beta2Reason,
		})

		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryAPIServerWaitingForVMProvisionedV1Beta2Reason,
		})
	}

	// Check if the infrastructure is ready, otherwise return and wait for the cluster object to be updated
	if cluster.Status.Initialization == nil || !cluster.Status.Initialization.InfrastructureProvisioned {
		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryVMWaitingForClusterInfrastructureV1Beta2Reason,
		})
		setOtherWaitingConditions()
		log.Info("Waiting for InMemoryCluster Controller to create cluster infrastructure")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	// NOTE: we are not using bootstrap data, but we wait for it in order to simulate a real machine
	// provisioning workflow.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
			v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingControlPlaneInitializedReason, clusterv1.ConditionSeverityInfo, "")
			conditions.Set(inMemoryMachine, metav1.Condition{
				Type:   infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: infrav1.DevMachineInMemoryVMWaitingForControlPlaneInitializedV1Beta2Reason,
			})
			setOtherWaitingConditions()
			log.Info("Waiting for the control plane to be initialized")
			return ctrl.Result{}, nil
		}

		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryVMWaitingForBootstrapDataV1Beta2Reason,
		})
		setOtherWaitingConditions()
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	// Call the inner reconciliation methods.
	phases := []func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error){
		r.reconcileNormalCloudMachine,
		r.reconcileNormalNode,
		r.reconcileNormalETCD,
		r.reconcileNormalAPIServer,
		r.reconcileNormalScheduler,
		r.reconcileNormalControllerManager,
		r.reconcileNormalKubeadmObjects,
		r.reconcileNormalKubeProxy,
		r.reconcileNormalCoredns,
	}

	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		phaseResult, err := phase(ctx, cluster, machine, inMemoryMachine)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		// TODO: consider if we have to use max(RequeueAfter) instead of min(RequeueAfter) to reduce the pressure on
		//  the reconcile queue for InMemoryMachines given that we are requeuing just to wait for some period to expire;
		//  the downside of it is that InMemoryMachines status will change by "big steps" vs incrementally.
		res = util.LowestNonZeroResult(res, phaseResult)
	}
	return res, kerrors.NewAggregate(errs)
}

func (r *MachineBackendReconciler) reconcileNormalCloudMachine(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (_ ctrl.Result, retErr error) {
	defer func() {
		if retErr != nil {
			conditions.Set(inMemoryMachine, metav1.Condition{
				Type:    infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineInMemoryVMInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			})
		}
	}()

	// Compute the name for resource group.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create VM; a Cloud VM can be created as soon as the Infra Machine is created
	// NOTE: for sake of simplicity we keep in memory resources as global resources (namespace empty).
	cloudMachine := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
	}
	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(cloudMachine), cloudMachine); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if err := inmemoryClient.Create(ctx, cloudMachine); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create CloudMachine")
		}
	}

	// Wait for the VM to be provisioned; provisioned happens a configurable time after the cloud machine creation.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Backend.InMemory.VM != nil {
		x := inMemoryMachine.Spec.Backend.InMemory.VM.Provisioning

		provisioningDuration = x.StartupDuration.Duration
		if x.StartupJitter != "" {
			jitter, err := strconv.ParseFloat(x.StartupJitter, 64)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to parse VM's StartupJitter")
			}
			if jitter > 0.0 {
				provisioningDuration += time.Duration(rand.Float64() * jitter * float64(provisioningDuration)) //nolint:gosec // Intentionally using a weak random number generator here.
			}
		}
	}

	start := cloudMachine.CreationTimestamp
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.VMWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryVMWaitingForStartupTimeoutV1Beta2Reason,
		})
		return ctrl.Result{RequeueAfter: start.Add(provisioningDuration).Sub(now)}, nil
	}

	// TODO: consider if to surface VM provisioned also on the cloud machine (currently it surfaces only on the inMemoryMachine)

	inMemoryMachine.Spec.ProviderID = ptr.To(calculateProviderID(inMemoryMachine))
	inMemoryMachine.Status.Ready = true
	v1beta1conditions.MarkTrue(inMemoryMachine, infrav1.VMProvisionedCondition)
	conditions.Set(inMemoryMachine, metav1.Condition{
		Type:   infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevMachineInMemoryVMProvisionedV1Beta2Reason,
	})
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalNode(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (_ ctrl.Result, retErr error) {
	// No-op if the VM is not provisioned yet
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.VMProvisionedCondition) {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryNodeWaitingForVMProvisionedV1Beta2Reason,
		})
		return ctrl.Result{}, nil
	}

	defer func() {
		if retErr != nil {
			conditions.Set(inMemoryMachine, metav1.Condition{
				Type:    infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineInMemoryNodeInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			})
		}
	}()

	// Wait for the node/kubelet to start up; node/kubelet start happens a configurable time after the VM is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Backend.InMemory.Node != nil {
		x := inMemoryMachine.Spec.Backend.InMemory.Node.Provisioning

		provisioningDuration = x.StartupDuration.Duration
		if x.StartupJitter != "" {
			jitter, err := strconv.ParseFloat(x.StartupJitter, 64)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to parse node's StartupJitter")
			}
			if jitter > 0.0 {
				provisioningDuration += time.Duration(rand.Float64() * jitter * float64(provisioningDuration)) //nolint:gosec // Intentionally using a weak random number generator here.
			}
		}
	}

	// TODO (v1beta2): test for v1beta2 conditions
	start := v1beta1conditions.Get(inMemoryMachine, infrav1.VMProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.NodeProvisionedCondition, infrav1.NodeWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryNodeWaitingForStartupTimeoutV1Beta2Reason,
		})
		return ctrl.Result{RequeueAfter: start.Add(provisioningDuration).Sub(now)}, nil
	}

	// Compute the name for resource group.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create Node
	// TODO: consider if to handle an additional setting adding a delay in between create node and node ready/provider ID being set
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
		Spec: corev1.NodeSpec{
			ProviderID: calculateProviderID(inMemoryMachine),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					LastTransitionTime: metav1.Now(),
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					Reason:             "KubeletReady",
				},
				{
					LastTransitionTime: metav1.Now(),
					Type:               corev1.NodeMemoryPressure,
					Status:             corev1.ConditionFalse,
					Reason:             "KubeletHasSufficientMemory",
				},
				{
					LastTransitionTime: metav1.Now(),
					Type:               corev1.NodeDiskPressure,
					Status:             corev1.ConditionFalse,
					Reason:             "KubeletHasNoDiskPressure",
				},
				{
					LastTransitionTime: metav1.Now(),
					Type:               corev1.NodePIDPressure,
					Status:             corev1.ConditionFalse,
					Reason:             "KubeletHasSufficientPID",
				},
			},
		},
	}
	if util.IsControlPlaneMachine(machine) {
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Labels["node-role.kubernetes.io/control-plane"] = ""
	}

	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(node), node); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get node")
		}

		// NOTE: for the first control plane machine we might create the node before etcd and API server pod are running
		// but this is not an issue, because it won't be visible to CAPI until the API server start serving requests.
		if err := inmemoryClient.Create(ctx, node); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create Node")
		}
	}

	v1beta1conditions.MarkTrue(inMemoryMachine, infrav1.NodeProvisionedCondition)
	conditions.Set(inMemoryMachine, metav1.Condition{
		Type:   infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Reason,
	})
	return ctrl.Result{}, nil
}

func calculateProviderID(inMemoryMachine *infrav1.DevMachine) string {
	return fmt.Sprintf("in-memory://%s", inMemoryMachine.Name)
}

func (r *MachineBackendReconciler) reconcileNormalETCD(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (_ ctrl.Result, retErr error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// No-op if the VM is not provisioned yet
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.VMProvisionedCondition) {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryEtcdWaitingForVMProvisionedV1Beta2Reason,
		})
		return ctrl.Result{}, nil
	}

	// No-op if the Node is not provisioned yet
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.NodeProvisionedCondition) {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryEtcdWaitingForNodeProvisionedV1Beta2Reason,
		})
		return ctrl.Result{}, nil
	}

	defer func() {
		if retErr != nil {
			conditions.Set(inMemoryMachine, metav1.Condition{
				Type:    infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineInMemoryEtcdInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			})
		}
	}()

	// Wait for the etcd pod to start up; etcd pod start happens a configurable time after the Node is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Backend.InMemory.Etcd != nil {
		x := inMemoryMachine.Spec.Backend.InMemory.Etcd.Provisioning

		provisioningDuration = x.StartupDuration.Duration
		if x.StartupJitter != "" {
			jitter, err := strconv.ParseFloat(x.StartupJitter, 64)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to parse etcd's StartupJitter")
			}
			if jitter > 0.0 {
				provisioningDuration += time.Duration(rand.Float64() * jitter * float64(provisioningDuration)) //nolint:gosec // Intentionally using a weak random number generator here.
			}
		}
	}

	// TODO (v1beta2): test for v1beta2 conditions
	start := v1beta1conditions.Get(inMemoryMachine, infrav1.NodeProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.EtcdProvisionedCondition, infrav1.EtcdWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryEtcdWaitingForStartupTimeoutV1Beta2Reason,
		})
		return ctrl.Result{RequeueAfter: start.Add(provisioningDuration).Sub(now)}, nil
	}

	// Compute the resource group and listener unique name.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create the etcd pod
	// TODO: consider if to handle an additional setting adding a delay in between create pod and pod ready
	etcdMember := fmt.Sprintf("etcd-%s", inMemoryMachine.Name)
	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      etcdMember,
			Labels: map[string]string{
				"component": "etcd",
				"tier":      "control-plane",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: inMemoryMachine.Name,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(etcdPod), etcdPod); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get etcd Pod")
		}

		// Gets info about the current etcd cluster, if any.
		info, err := r.getEtcdInfo(ctx, inmemoryClient)
		if err != nil {
			return ctrl.Result{}, err
		}

		// If this is the first etcd member in the cluster, assign a cluster ID
		if info.clusterID == "" {
			for {
				info.clusterID = fmt.Sprintf("%d", rand.Uint32()) //nolint:gosec // weak random number generator is good enough here
				if info.clusterID != "0" {
					break
				}
			}
		}

		// Computes a unique memberID.
		var memberID string
		for {
			memberID = fmt.Sprintf("%d", rand.Uint32()) //nolint:gosec // weak random number generator is good enough here
			if !info.members.Has(memberID) && memberID != "0" {
				break
			}
		}

		// Annotate the pod with the info about the etcd cluster.
		etcdPod.Annotations = map[string]string{
			cloudv1.EtcdClusterIDAnnotationName: info.clusterID,
			cloudv1.EtcdMemberIDAnnotationName:  memberID,
		}

		// If the etcd cluster is being created it doesn't have a leader yet, so set this member as a leader.
		if info.leaderID == "" {
			etcdPod.Annotations[cloudv1.EtcdLeaderFromAnnotationName] = time.Now().Format(time.RFC3339)
		}

		// NOTE: for the first control plane machine we might create the etcd pod before the API server pod is running
		// but this is not an issue, because it won't be visible to CAPI until the API server start serving requests.
		if err := inmemoryClient.Create(ctx, etcdPod); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create Pod")
		}
	}

	// If there is not yet an etcd member listener for this machine, add it to the server.
	if !r.APIServerMux.HasEtcdMember(listenerName, etcdMember) {
		// Getting the etcd CA
		s, err := secret.Get(ctx, r.Client, client.ObjectKeyFromObject(cluster), secret.EtcdCA)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get etcd CA")
		}
		certData, exists := s.Data[secret.TLSCrtDataName]
		if !exists {
			return ctrl.Result{}, errors.Errorf("invalid etcd CA: missing data for %s", secret.TLSCrtDataName)
		}

		cert, err := certs.DecodeCertPEM(certData)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "invalid etcd CA: invalid %s", secret.TLSCrtDataName)
		}

		keyData, exists := s.Data[secret.TLSKeyDataName]
		if !exists {
			return ctrl.Result{}, errors.Errorf("invalid etcd CA: missing data for %s", secret.TLSKeyDataName)
		}

		key, err := certs.DecodePrivateKeyPEM(keyData)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "invalid etcd CA: invalid %s", secret.TLSKeyDataName)
		}

		if err := r.APIServerMux.AddEtcdMember(listenerName, etcdMember, cert, key.(*rsa.PrivateKey)); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to start etcd member")
		}
	}

	v1beta1conditions.MarkTrue(inMemoryMachine, infrav1.EtcdProvisionedCondition)
	conditions.Set(inMemoryMachine, metav1.Condition{
		Type:   infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Reason,
	})
	return ctrl.Result{}, nil
}

type etcdInfo struct {
	clusterID string
	leaderID  string
	members   sets.Set[string]
}

func (r *MachineBackendReconciler) getEtcdInfo(ctx context.Context, inmemoryClient inmemoryruntime.Client) (etcdInfo, error) {
	etcdPods := &corev1.PodList{}
	if err := inmemoryClient.List(ctx, etcdPods,
		client.InNamespace(metav1.NamespaceSystem),
		client.MatchingLabels{
			"component": "etcd",
			"tier":      "control-plane"},
	); err != nil {
		return etcdInfo{}, errors.Wrap(err, "failed to list etcd members")
	}

	if len(etcdPods.Items) == 0 {
		return etcdInfo{}, nil
	}

	info := etcdInfo{
		members: sets.New[string](),
	}
	var leaderFrom time.Time
	for _, pod := range etcdPods.Items {
		if _, ok := pod.Annotations[cloudv1.EtcdMemberRemoved]; ok {
			continue
		}
		if info.clusterID == "" {
			info.clusterID = pod.Annotations[cloudv1.EtcdClusterIDAnnotationName]
		} else if pod.Annotations[cloudv1.EtcdClusterIDAnnotationName] != info.clusterID {
			return etcdInfo{}, errors.New("invalid etcd cluster, members have different cluster ID")
		}
		memberID := pod.Annotations[cloudv1.EtcdMemberIDAnnotationName]
		info.members.Insert(memberID)

		if t, err := time.Parse(time.RFC3339, pod.Annotations[cloudv1.EtcdLeaderFromAnnotationName]); err == nil {
			if t.After(leaderFrom) {
				info.leaderID = memberID
				leaderFrom = t
			}
		}
	}

	if info.leaderID == "" {
		// TODO: consider if and how to automatically recover from this case
		//  note: this can happen also when reading etcd members in the server, might be it is something we have to take case before deletion...
		//  for now it should not be an issue because KCP forward etcd leadership before deletion.
		return etcdInfo{}, errors.New("invalid etcd cluster, no leader found")
	}

	return info, nil
}

func (r *MachineBackendReconciler) reconcileNormalAPIServer(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (_ ctrl.Result, retErr error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// No-op if the VM is not provisioned yet
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.VMProvisionedCondition) {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryAPIServerWaitingForVMProvisionedV1Beta2Reason,
		})
		return ctrl.Result{}, nil
	}

	// No-op if the Node is not provisioned yet
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.NodeProvisionedCondition) {
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryAPIServerWaitingForNodeProvisionedV1Beta2Reason,
		})
		return ctrl.Result{}, nil
	}

	defer func() {
		if retErr != nil {
			conditions.Set(inMemoryMachine, metav1.Condition{
				Type:    infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.DevMachineInMemoryAPIServerInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			})
		}
	}()

	// Wait for the API server pod to start up; API server pod start happens a configurable time after the Node is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Backend.InMemory.APIServer != nil {
		x := inMemoryMachine.Spec.Backend.InMemory.APIServer.Provisioning

		provisioningDuration = x.StartupDuration.Duration
		if x.StartupJitter != "" {
			jitter, err := strconv.ParseFloat(x.StartupJitter, 64)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to parse API server's StartupJitter")
			}
			if jitter > 0.0 {
				provisioningDuration += time.Duration(rand.Float64() * jitter * float64(provisioningDuration)) //nolint:gosec // Intentionally using a weak random number generator here.
			}
		}
	}

	// TODO (v1beta2): test for v1beta2 conditions
	start := v1beta1conditions.Get(inMemoryMachine, infrav1.NodeProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		v1beta1conditions.MarkFalse(inMemoryMachine, infrav1.APIServerProvisionedCondition, infrav1.APIServerWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
		conditions.Set(inMemoryMachine, metav1.Condition{
			Type:   infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: infrav1.DevMachineInMemoryAPIServerWaitingForStartupTimeoutV1Beta2Reason,
		})
		return ctrl.Result{RequeueAfter: start.Add(provisioningDuration).Sub(now)}, nil
	}

	// Compute the resource group and listener unique name.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create the apiserver pod
	// TODO: consider if to handle an additional setting adding a delay in between create pod and pod ready
	apiServer := fmt.Sprintf("kube-apiserver-%s", inMemoryMachine.Name)

	apiServerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      apiServer,
			Labels: map[string]string{
				"component": "kube-apiserver",
				"tier":      "control-plane",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: inMemoryMachine.Name,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(apiServerPod), apiServerPod); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get apiServer Pod")
		}

		if err := inmemoryClient.Create(ctx, apiServerPod); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create apiServer Pod")
		}
	}

	// If there is not yet an API server listener for this machine.
	if !r.APIServerMux.HasAPIServer(listenerName, apiServer) {
		// Getting the Kubernetes CA
		s, err := secret.Get(ctx, r.Client, client.ObjectKeyFromObject(cluster), secret.ClusterCA)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get cluster CA")
		}
		certData, exists := s.Data[secret.TLSCrtDataName]
		if !exists {
			return ctrl.Result{}, errors.Errorf("invalid cluster CA: missing data for %s", secret.TLSCrtDataName)
		}

		cert, err := certs.DecodeCertPEM(certData)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "invalid cluster CA: invalid %s", secret.TLSCrtDataName)
		}

		keyData, exists := s.Data[secret.TLSKeyDataName]
		if !exists {
			return ctrl.Result{}, errors.Errorf("invalid cluster CA: missing data for %s", secret.TLSKeyDataName)
		}

		key, err := certs.DecodePrivateKeyPEM(keyData)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "invalid cluster CA: invalid %s", secret.TLSKeyDataName)
		}

		// Adding the APIServer.
		// NOTE: When the first APIServer is added, the workload cluster listener is started.
		if err := r.APIServerMux.AddAPIServer(listenerName, apiServer, cert, key.(*rsa.PrivateKey)); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to start API server")
		}
	}

	v1beta1conditions.MarkTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition)
	conditions.Set(inMemoryMachine, metav1.Condition{
		Type:   infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Reason,
	})
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalScheduler(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// NOTE: we are creating the scheduler pod to make KCP happy, but we are not implementing any
	// specific behaviour for this component because they are not relevant for stress tests.
	// As a current approximation, we create the scheduler as soon as the API server is provisioned;
	// also, the scheduler is immediately marked as ready.
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	schedulerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-scheduler-%s", inMemoryMachine.Name),
			Labels: map[string]string{
				"component": "kube-scheduler",
				"tier":      "control-plane",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: inMemoryMachine.Name,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if err := inmemoryClient.Create(ctx, schedulerPod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create scheduler Pod")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalControllerManager(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// NOTE: we are creating the controller manager pod to make KCP happy, but we are not implementing any
	// specific behaviour for this component because they are not relevant for stress tests.
	// As a current approximation, we create the controller manager as soon as the API server is provisioned;
	// also, the controller manager is immediately marked as ready.
	// TODO (v1beta2): test for v1beta2 conditions
	if !v1beta1conditions.IsTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	controllerManagerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-controller-manager-%s", inMemoryMachine.Name),
			Labels: map[string]string{
				"component": "kube-controller-manager",
				"tier":      "control-plane",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: inMemoryMachine.Name,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if err := inmemoryClient.Create(ctx, controllerManagerPod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create controller manager Pod")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalKubeadmObjects(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// create kubeadm ClusterRole and ClusterRoleBinding enforced by KCP
	// NOTE: we create those objects because this is what kubeadm does, but KCP creates
	// ClusterRole and ClusterRoleBinding if not found.

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeadm:get-nodes",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get"},
				APIGroups: []string{""},
				Resources: []string{"nodes"},
			},
		},
	}
	if err := inmemoryClient.Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create kubeadm:get-nodes ClusterRole")
	}

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeadm:get-nodes",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "kubeadm:get-nodes",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.GroupKind,
				Name: "system:bootstrappers:kubeadm:default-node-token",
			},
		},
	}
	if err := inmemoryClient.Create(ctx, roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create kubeadm:get-nodes ClusterRoleBinding")
	}

	// create kubeadm config map
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": "",
		},
	}
	if err := inmemoryClient.Create(ctx, cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create kubeadm-config ConfigMap")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalKubeProxy(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// TODO: Add provisioning time for KubeProxy.

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create the kube-proxy-daemonset
	kubeProxyDaemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "kube-proxy",
			Labels: map[string]string{
				"component": "kube-proxy",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kube-proxy",
							Image: fmt.Sprintf("registry.k8s.io/kube-proxy:%s", *machine.Spec.Version),
						},
					},
				},
			},
		},
	}
	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(kubeProxyDaemonSet), kubeProxyDaemonSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get kube-proxy DaemonSet")
		}

		if err := inmemoryClient.Create(ctx, kubeProxyDaemonSet); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create kube-proxy DaemonSet")
		}
	}
	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileNormalCoredns(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// TODO: Add provisioning time for CoreDNS.

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create the coredns configMap.
	corednsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "coredns",
		},
		Data: map[string]string{
			"Corefile": "ANG",
		},
	}
	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(corednsConfigMap), corednsConfigMap); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get coreDNS configMap")
		}

		if err := inmemoryClient.Create(ctx, corednsConfigMap); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create coreDNS configMap")
		}
	}
	// Create the coredns deployment.
	corednsDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "coredns",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "coredns",
							Image: "registry.k8s.io/coredns/coredns:v1.10.1",
						},
					},
				},
			},
		},
	}

	if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(corednsDeployment), corednsDeployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get coreDNS deployment")
		}

		if err := inmemoryClient.Create(ctx, corednsDeployment); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create coreDNS deployment")
		}
	}
	return ctrl.Result{}, nil
}

// ReconcileDelete handle in memory backend for deleted DevMachine.
func (r *MachineBackendReconciler) ReconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.DevCluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	if inMemoryMachine.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevMachines without an InMemory backend")
	}
	if inMemoryCluster.Spec.Backend.InMemory == nil {
		return ctrl.Result{}, errors.New("InMemoryBackendReconciler can't be called for DevCluster without an InMemory backend")
	}

	// Call the inner reconciliation methods.
	phases := []func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error){
		// TODO: revisit order when we implement behaviour for the deletion workflow
		r.reconcileDeleteNode,
		r.reconcileDeleteETCD,
		r.reconcileDeleteAPIServer,
		r.reconcileDeleteScheduler,
		r.reconcileDeleteControllerManager,
		r.reconcileDeleteCloudMachine,
		// Note: We are not deleting kubeadm objects because they exist in K8s, they are not related to a specific machine.
	}

	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		phaseResult, err := phase(ctx, cluster, machine, inMemoryMachine)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}
	if res.IsZero() && len(errs) == 0 {
		controllerutil.RemoveFinalizer(inMemoryMachine, infrav1.MachineFinalizer)
	}
	return res, kerrors.NewAggregate(errs)
}

func (r *MachineBackendReconciler) reconcileDeleteCloudMachine(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Delete VM
	cloudMachine := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
	}
	if err := inmemoryClient.Delete(ctx, cloudMachine); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete CloudMachine")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileDeleteNode(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Delete Node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
	}

	// TODO(killianmuldoon): check if we can drop this given that the MachineController is already draining pods and deleting nodes.
	if err := inmemoryClient.Delete(ctx, node); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete Node")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileDeleteETCD(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group and listener unique name.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	etcdMember := fmt.Sprintf("etcd-%s", inMemoryMachine.Name)
	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      etcdMember,
		},
	}
	if err := inmemoryClient.Delete(ctx, etcdPod); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete etcd Pod")
	}
	if err := r.APIServerMux.DeleteEtcdMember(listenerName, etcdMember); err != nil {
		return ctrl.Result{}, err
	}

	// TODO: if all the etcd members are gone, cleanup all the k8s objects from the resource group.
	// note: it is not possible to delete the resource group, because cloud resources should be preserved.
	// given that, in order to implement this it is required to find a way to identify all the k8s resources (might be via gvk);
	// also, deletion must happen suddenly, without respecting finalizers or owner references links.

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileDeleteAPIServer(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group and listener unique name.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	apiServer := fmt.Sprintf("kube-apiserver-%s", inMemoryMachine.Name)
	apiServerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      apiServer,
		},
	}
	if err := inmemoryClient.Delete(ctx, apiServerPod); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete apiServer Pod")
	}
	if err := r.APIServerMux.DeleteAPIServer(listenerName, apiServer); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileDeleteScheduler(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	schedulerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-scheduler-%s", inMemoryMachine.Name),
		},
	}
	if err := inmemoryClient.Delete(ctx, schedulerPod); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to scheduler Pod")
	}

	return ctrl.Result{}, nil
}

func (r *MachineBackendReconciler) reconcileDeleteControllerManager(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.DevMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()
	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	controllerManagerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-controller-manager-%s", inMemoryMachine.Name),
		},
	}
	if err := inmemoryClient.Delete(ctx, controllerManagerPod); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to controller manager Pod")
	}

	return ctrl.Result{}, nil
}

// PatchDevMachine patch a DevMachine.
func (r *MachineBackendReconciler) PatchDevMachine(ctx context.Context, patchHelper *patch.Helper, inMemoryMachine *infrav1.DevMachine, isControlPlane bool) error {
	if inMemoryMachine.Spec.Backend.InMemory == nil {
		return errors.New("InMemoryBackendReconciler can't be called for DevMachines without an InMemory backend")
	}

	inMemoryMachineConditions := []clusterv1.ConditionType{
		infrav1.VMProvisionedCondition,
		infrav1.NodeProvisionedCondition,
	}
	inMemoryMachineV1Beta2Conditions := conditions.ForConditionTypes{
		infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
		infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
	}
	if isControlPlane {
		inMemoryMachineConditions = append(inMemoryMachineConditions,
			infrav1.EtcdProvisionedCondition,
			infrav1.APIServerProvisionedCondition,
		)
		inMemoryMachineV1Beta2Conditions = append(inMemoryMachineV1Beta2Conditions,
			infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
		)
	}
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding the step counter during the deletion process).
	v1beta1conditions.SetSummary(inMemoryMachine,
		v1beta1conditions.WithConditions(inMemoryMachineConditions...),
		v1beta1conditions.WithStepCounterIf(inMemoryMachine.DeletionTimestamp.IsZero() && inMemoryMachine.Spec.ProviderID == nil),
	)
	if err := conditions.SetSummaryCondition(inMemoryMachine, inMemoryMachine, infrav1.DevMachineReadyV1Beta2Condition,
		inMemoryMachineV1Beta2Conditions,
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				// Use custom reasons.
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					infrav1.DevMachineNotReadyV1Beta2Reason,
					infrav1.DevMachineReadyUnknownV1Beta2Reason,
					infrav1.DevMachineReadyV1Beta2Reason,
				)),
			),
		},
	); err != nil {
		return errors.Wrapf(err, "failed to set %s condition", infrav1.DevMachineReadyV1Beta2Condition)
	}

	return patchHelper.Patch(ctx, inMemoryMachine,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyV1Beta1Condition,
			infrav1.VMProvisionedCondition,
			infrav1.NodeProvisionedCondition,
			infrav1.EtcdProvisionedCondition,
			infrav1.APIServerProvisionedCondition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			infrav1.DevMachineReadyV1Beta2Condition,
			infrav1.DevMachineInMemoryVMProvisionedV1Beta2Condition,
			infrav1.DevMachineInMemoryNodeProvisionedV1Beta2Condition,
			infrav1.DevMachineInMemoryEtcdProvisionedV1Beta2Condition,
			infrav1.DevMachineInMemoryAPIServerProvisionedV1Beta2Condition,
		}},
	)
}
