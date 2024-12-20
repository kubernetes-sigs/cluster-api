/*
Copyright 2023 The Kubernetes Authors.

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

// Package controllers implements controller functionality.
package controllers

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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
)

// InMemoryMachineReconciler reconciles a InMemoryMachine object.
type InMemoryMachineReconciler struct {
	client.Client
	InMemoryManager inmemoryruntime.Manager
	APIServerMux    *inmemoryserver.WorkloadClustersMux

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachines/status;inmemorymachines/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles InMemoryMachine events.
func (r *InMemoryMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	// Fetch the InMemoryMachine instance
	inMemoryMachine := &infrav1.InMemoryMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, inMemoryMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, inMemoryMachine, infrav1.MachineFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of InMemoryMachine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, inMemoryMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, inMemoryMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on InMemoryMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("InMemoryMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, inMemoryMachine); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	// Fetch the in-memory Cluster.
	inMemoryCluster := &infrav1.InMemoryCluster{}
	inMemoryClusterName := client.ObjectKey{
		Namespace: inMemoryMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, inMemoryClusterName, inMemoryCluster); err != nil {
		log.Info("InMemoryCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(inMemoryMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the InMemoryMachine object and status after each reconciliation.
	defer func() {
		inMemoryMachineConditions := []clusterv1.ConditionType{
			infrav1.VMProvisionedCondition,
			infrav1.NodeProvisionedCondition,
		}
		if util.IsControlPlaneMachine(machine) {
			inMemoryMachineConditions = append(inMemoryMachineConditions,
				infrav1.EtcdProvisionedCondition,
				infrav1.APIServerProvisionedCondition,
			)
		}
		// Always update the readyCondition by summarizing the state of other conditions.
		// A step counter is added to represent progress during the provisioning process (instead we are hiding the step counter during the deletion process).
		conditions.SetSummary(inMemoryMachine,
			conditions.WithConditions(inMemoryMachineConditions...),
			conditions.WithStepCounterIf(inMemoryMachine.ObjectMeta.DeletionTimestamp.IsZero() && inMemoryMachine.Spec.ProviderID == nil),
		)
		if err := patchHelper.Patch(ctx, inMemoryMachine, patch.WithOwnedConditions{Conditions: inMemoryMachineConditions}); err != nil {
			rerr = kerrors.NewAggregate([]error{rerr, err})
		}
	}()

	// Handle deleted machines
	if !inMemoryMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, machine, inMemoryMachine)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machine, inMemoryMachine)
}

func (r *InMemoryMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check if the infrastructure is ready, otherwise return and wait for the cluster object to be updated
	if !cluster.Status.InfrastructureReady {
		conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		log.Info("Waiting for InMemoryCluster Controller to create cluster infrastructure")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	// NOTE: we are not using bootstrap data, but we wait for it in order to simulate a real machine
	// provisioning workflow.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
			conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingControlPlaneInitializedReason, clusterv1.ConditionSeverityInfo, "")
			log.Info("Waiting for the control plane to be initialized")
			return ctrl.Result{}, nil
		}

		conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	// Call the inner reconciliation methods.
	phases := []func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error){
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

func (r *InMemoryMachineReconciler) reconcileNormalCloudMachine(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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
	if inMemoryMachine.Spec.Behaviour != nil && inMemoryMachine.Spec.Behaviour.VM != nil {
		x := inMemoryMachine.Spec.Behaviour.VM.Provisioning

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
		conditions.MarkFalse(inMemoryMachine, infrav1.VMProvisionedCondition, infrav1.VMWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{RequeueAfter: start.Add(provisioningDuration).Sub(now)}, nil
	}

	// TODO: consider if to surface VM provisioned also on the cloud machine (currently it surfaces only on the inMemoryMachine)

	inMemoryMachine.Spec.ProviderID = ptr.To(calculateProviderID(inMemoryMachine))
	inMemoryMachine.Status.Ready = true
	conditions.MarkTrue(inMemoryMachine, infrav1.VMProvisionedCondition)
	return ctrl.Result{}, nil
}

func (r *InMemoryMachineReconciler) reconcileNormalNode(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// No-op if the VM is not provisioned yet
	if !conditions.IsTrue(inMemoryMachine, infrav1.VMProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Wait for the node/kubelet to start up; node/kubelet start happens a configurable time after the VM is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Behaviour != nil && inMemoryMachine.Spec.Behaviour.Node != nil {
		x := inMemoryMachine.Spec.Behaviour.Node.Provisioning

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

	start := conditions.Get(inMemoryMachine, infrav1.VMProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		conditions.MarkFalse(inMemoryMachine, infrav1.NodeProvisionedCondition, infrav1.NodeWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
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

	conditions.MarkTrue(inMemoryMachine, infrav1.NodeProvisionedCondition)
	return ctrl.Result{}, nil
}

func calculateProviderID(inMemoryMachine *infrav1.InMemoryMachine) string {
	return fmt.Sprintf("in-memory://%s", inMemoryMachine.Name)
}

func (r *InMemoryMachineReconciler) reconcileNormalETCD(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// No-op if the Node is not provisioned yet
	if !conditions.IsTrue(inMemoryMachine, infrav1.NodeProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Wait for the etcd pod to start up; etcd pod start happens a configurable time after the Node is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Behaviour != nil && inMemoryMachine.Spec.Behaviour.Etcd != nil {
		x := inMemoryMachine.Spec.Behaviour.Etcd.Provisioning

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

	start := conditions.Get(inMemoryMachine, infrav1.NodeProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		conditions.MarkFalse(inMemoryMachine, infrav1.EtcdProvisionedCondition, infrav1.EtcdWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
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

	conditions.MarkTrue(inMemoryMachine, infrav1.EtcdProvisionedCondition)
	return ctrl.Result{}, nil
}

type etcdInfo struct {
	clusterID string
	leaderID  string
	members   sets.Set[string]
}

func (r *InMemoryMachineReconciler) getEtcdInfo(ctx context.Context, inmemoryClient inmemoryruntime.Client) (etcdInfo, error) {
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

func (r *InMemoryMachineReconciler) reconcileNormalAPIServer(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// No-op if the Node is not provisioned yet
	if !conditions.IsTrue(inMemoryMachine, infrav1.NodeProvisionedCondition) {
		return ctrl.Result{}, nil
	}

	// Wait for the API server pod to start up; API server pod start happens a configurable time after the Node is provisioned.
	provisioningDuration := time.Duration(0)
	if inMemoryMachine.Spec.Behaviour != nil && inMemoryMachine.Spec.Behaviour.APIServer != nil {
		x := inMemoryMachine.Spec.Behaviour.APIServer.Provisioning

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

	start := conditions.Get(inMemoryMachine, infrav1.NodeProvisionedCondition).LastTransitionTime
	now := time.Now()
	if now.Before(start.Add(provisioningDuration)) {
		conditions.MarkFalse(inMemoryMachine, infrav1.APIServerProvisionedCondition, infrav1.APIServerWaitingForStartupTimeoutReason, clusterv1.ConditionSeverityInfo, "")
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

	conditions.MarkTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition)
	return ctrl.Result{}, nil
}

func (r *InMemoryMachineReconciler) reconcileNormalScheduler(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// NOTE: we are creating the scheduler pod to make KCP happy, but we are not implementing any
	// specific behaviour for this component because they are not relevant for stress tests.
	// As a current approximation, we create the scheduler as soon as the API server is provisioned;
	// also, the scheduler is immediately marked as ready.
	if !conditions.IsTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition) {
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

func (r *InMemoryMachineReconciler) reconcileNormalControllerManager(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// No-op if the machine is not a control plane machine.
	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// NOTE: we are creating the controller manager pod to make KCP happy, but we are not implementing any
	// specific behaviour for this component because they are not relevant for stress tests.
	// As a current approximation, we create the controller manager as soon as the API server is provisioned;
	// also, the controller manager is immediately marked as ready.
	if !conditions.IsTrue(inMemoryMachine, infrav1.APIServerProvisionedCondition) {
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

func (r *InMemoryMachineReconciler) reconcileNormalKubeadmObjects(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileNormalKubeProxy(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileNormalCoredns(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, _ *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// Call the inner reconciliation methods.
	phases := []func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error){
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

func (r *InMemoryMachineReconciler) reconcileDeleteCloudMachine(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDeleteNode(ctx context.Context, cluster *clusterv1.Cluster, _ *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDeleteETCD(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDeleteAPIServer(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDeleteScheduler(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

func (r *InMemoryMachineReconciler) reconcileDeleteControllerManager(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
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

// SetupWithManager will add watches for this controller.
func (r *InMemoryMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.InMemoryManager == nil || r.APIServerMux == nil {
		return errors.New("Client, InMemoryManager and APIServerMux must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "inmemorymachine")
	clusterToInMemoryMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.InMemoryMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InMemoryMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("InMemoryMachine"))),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&infrav1.InMemoryCluster{},
			handler.EnqueueRequestsFromMapFunc(r.InMemoryClusterToInMemoryMachines),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToInMemoryMachines),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), predicateLog),
			)),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// InMemoryClusterToInMemoryMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of InMemoryMachines.
func (r *InMemoryMachineReconciler) InMemoryClusterToInMemoryMachines(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.(*infrav1.InMemoryCluster)
	if !ok {
		panic(fmt.Sprintf("Expected a InMemoryCluster but got a %T", o))
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}
