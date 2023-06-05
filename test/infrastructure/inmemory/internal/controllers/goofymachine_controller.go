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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
)

// InMemoryMachineReconciler reconciles a InMemoryMachine object.
type InMemoryMachineReconciler struct {
	client.Client
	CloudManager cloud.Manager
	APIServerMux *server.WorkloadClustersMux

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachines/status;inmemorymachines/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles InMemoryMachine events.
func (r *InMemoryMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the InMemoryMachine instance
	inMemoryMachine := &infrav1.InMemoryMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, inMemoryMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
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

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, inMemoryMachine) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
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
		if err := patchHelper.Patch(ctx, inMemoryMachine); err != nil {
			log.Error(err, "failed to patch InMemoryMachine")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(inMemoryMachine, infrav1.MachineFinalizer) {
		controllerutil.AddFinalizer(inMemoryMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	if !inMemoryMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, inMemoryMachine)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machine, inMemoryMachine)
}

func (r *InMemoryMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Compute the resource group unique name.
	// NOTE: We are using reconcilerGroup also as a name for the listener for sake of simplicity.
	resourceGroup := klog.KObj(cluster).String()
	cloudClient := r.CloudManager.GetResourceGroup(resourceGroup).GetClient()

	// Check if the infrastructure is ready, otherwise return and wait for the cluster object to be updated
	if !cluster.Status.InfrastructureReady {
		log.Info("Waiting for InMemoryCluster Controller to create cluster infrastructure")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	// NOTE: we are not using bootstrap data, but we wait for it in order to simulate a real machine
	// provisioning workflow.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
			log.Info("Waiting for the control plane to be initialized")
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	// Create VM
	// NOTE: for sake of simplicity we keep cloud resources as global resources (namespace empty).
	// TODO: we should convert this into a full reconcile (if it exist, update)
	cloudMachine := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
	}
	if err := cloudClient.Create(ctx, cloudMachine); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create CloudMachine")
	}

	// Create Node
	// TODO: we should convert this into a full reconcile (if it exist, update)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: inMemoryMachine.Name,
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("in-memory://%s", inMemoryMachine.Name),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
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
	if err := cloudClient.Create(ctx, node); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create Node")
	}

	// NOTE: this can probably go up, after the VM is created
	inMemoryMachine.Spec.ProviderID = &node.Spec.ProviderID
	inMemoryMachine.Status.Ready = true

	if !util.IsControlPlaneMachine(machine) {
		return ctrl.Result{}, nil
	}

	// If there is not yet an etcd member listener for this machine.
	etcdMember := fmt.Sprintf("etcd-%s", inMemoryMachine.Name)
	if !r.APIServerMux.HasEtcdMember(resourceGroup, etcdMember) {
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

		if err := r.APIServerMux.AddEtcdMember(resourceGroup, etcdMember, cert, key.(*rsa.PrivateKey)); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to start etcd member")
		}
	}

	// If there is not yet an API server listener for this machine.
	apiServer := fmt.Sprintf("kube-apiserver-%s", inMemoryMachine.Name)
	if !r.APIServerMux.HasAPIServer(resourceGroup, apiServer) {
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
		if err := r.APIServerMux.AddAPIServer(resourceGroup, apiServer, cert, key.(*rsa.PrivateKey)); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to start API server")
		}
	}

	// TBD is this is the right point in the sequence, the pod shows up after API server is running.

	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      etcdMember,
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
	if err := cloudClient.Get(ctx, client.ObjectKeyFromObject(etcdPod), etcdPod); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get etcdPod Pod")
		}

		etcdPod.Labels = map[string]string{
			"component": "etcd",
			"tier":      "control-plane",
		}
		etcdPod.Annotations = map[string]string{
			// TODO: read this from existing etcd pods, if any.
			"etcd.inmemory.infrastructure.cluster.x-k8s.io/cluster-id": fmt.Sprintf("%d", rand.Uint32()), //nolint:gosec // weak random number generator is good enough here
			"etcd.inmemory.infrastructure.cluster.x-k8s.io/member-id":  fmt.Sprintf("%d", rand.Uint32()), //nolint:gosec // weak random number generator is good enough here
			// TODO: set this only if there are no other leaders.
			"etcd.inmemory.infrastructure.cluster.x-k8s.io/leader-from": time.Now().Format(time.RFC3339),
		}

		if err := cloudClient.Create(ctx, etcdPod); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create etcdPod Pod")
		}
	}

	apiServerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      apiServer,
			Labels: map[string]string{
				"component": "kube-apiserver",
				"tier":      "control-plane",
			},
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
	if err := cloudClient.Create(ctx, apiServerPod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create apiServer Pod")
	}

	schedulerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-scheduler-%s", inMemoryMachine.Name),
			Labels: map[string]string{
				"component": "kube-scheduler",
				"tier":      "control-plane",
			},
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
	if err := cloudClient.Create(ctx, schedulerPod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create scheduler Pod")
	}

	controllerManagerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      fmt.Sprintf("kube-controller-manager-%s", inMemoryMachine.Name),
			Labels: map[string]string{
				"component": "kube-controller-manager",
				"tier":      "control-plane",
			},
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
	if err := cloudClient.Create(ctx, controllerManagerPod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create controller manager Pod")
	}

	// create kubeadm ClusterRole and ClusterRoleBinding enforced by KCP
	// TODO: drop this as soon as we implement CREATE

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeadm:get-nodes",
			// Namespace: metav1.NamespaceSystem, // TODO: drop in kubeadm? Yup!
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get"},
				APIGroups: []string{""},
				Resources: []string{"nodes"},
			},
		},
	}
	if err := cloudClient.Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create kubeadm:get-nodes ClusterRole")
	}

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeadm:get-nodes",
			// Namespace: metav1.NamespaceSystem, // TODO: drop in kubeadm? Yup!
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
	if err := cloudClient.Create(ctx, roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create kubeadm:get-nodes ClusterRoleBinding")
	}

	// create kubeadm config map
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
	}
	if err := cloudClient.Create(ctx, cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create ubeadm-config ConfigMap")
	}

	return ctrl.Result{}, nil
}

//nolint:unparam // once we implemented this func we will also return errors
func (r *InMemoryMachineReconciler) reconcileDelete(_ context.Context, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
	// TODO: implement
	controllerutil.RemoveFinalizer(inMemoryMachine, infrav1.MachineFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller.
func (r *InMemoryMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToInMemoryMachines, err := util.ClusterToObjectsMapper(mgr.GetClient(), &infrav1.InMemoryMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InMemoryMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("InMemoryMachine"))),
		).
		Watches(
			&infrav1.InMemoryCluster{},
			handler.EnqueueRequestsFromMapFunc(r.InMemoryClusterToInMemoryMachines),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToInMemoryMachines),
			builder.WithPredicates(
				predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			),
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
