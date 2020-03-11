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

package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	mhcClusterNameIndex  = "spec.clusterName"
	machineNodeNameIndex = "status.nodeRef.name"

	// Event types

	// EventRemediationRestricted is emitted in case when machine remediation
	// is restricted by remediation circuit shorting logic
	EventRemediationRestricted string = "RemediationRestricted"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinehealthchecks;machinehealthchecks/status,verbs=get;list;watch;update;patch

// MachineHealthCheckReconciler reconciles a MachineHealthCheck object
type MachineHealthCheckReconciler struct {
	Client client.Client
	Log    logr.Logger

	controller        controller.Controller
	recorder          record.EventRecorder
	scheme            *runtime.Scheme
	clusterCaches     map[client.ObjectKey]cache.Cache
	clusterCachesLock sync.RWMutex
}

func (r *MachineHealthCheckReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineHealthCheck{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.clusterToMachineHealthCheck)},
		).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.machineToMachineHealthCheck)},
		).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	// Add index to MachineHealthCheck for listing by Cluster Name
	if err := mgr.GetCache().IndexField(&clusterv1.MachineHealthCheck{},
		mhcClusterNameIndex,
		r.indexMachineHealthCheckByClusterName,
	); err != nil {
		return errors.Wrap(err, "error setting index fields")
	}

	// Add index to Machine for listing by Node reference
	if err := mgr.GetCache().IndexField(&clusterv1.Machine{},
		machineNodeNameIndex,
		r.indexMachineByNodeName,
	); err != nil {
		return errors.Wrap(err, "error setting index fields")
	}

	r.controller = controller
	r.recorder = mgr.GetEventRecorderFor("machinehealthcheck-controller")
	r.scheme = mgr.GetScheme()
	r.clusterCaches = make(map[client.ObjectKey]cache.Cache)
	return nil
}

func (r *MachineHealthCheckReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	logger := r.Log.WithValues("machinehealthcheck", req.Name, "namespace", req.Namespace)

	// Fetch the MachineHealthCheck instance
	m := &clusterv1.MachineHealthCheck{}
	if err := r.Client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to fetch MachineHealthCheck")
		return ctrl.Result{}, err
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		logger.Error(err, "Failed to fetch Cluster for MachineHealthCheck")
		return ctrl.Result{}, errors.Wrapf(err, "failed to get Cluster %q for MachineHealthCheck %q in namespace %q",
			m.Spec.ClusterName, m.Name, m.Namespace)
	}

	// Return early if the object or Cluster is paused.
	if util.IsPaused(cluster, m) {
		logger.V(3).Info("reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		logger.Error(err, "Failed to build patch helper")
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		if err := patchHelper.Patch(ctx, m); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Reconcile labels.
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterLabelName] = m.Spec.ClusterName

	result, err := r.reconcile(ctx, cluster, m)
	if err != nil {
		logger.Error(err, "Failed to reconcile MachineHealthCheck")
		r.recorder.Eventf(m, corev1.EventTypeWarning, "ReconcileError", "%v", err)

		// Requeue immediately if any errors occurred
		return ctrl.Result{}, err
	}

	return result, nil
}

func (r *MachineHealthCheckReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.MachineHealthCheck) (ctrl.Result, error) {
	// Ensure the MachineHealthCheck is owned by the Cluster it belongs to
	m.OwnerReferences = util.EnsureOwnerRef(m.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	})

	logger := r.Log.WithValues("machinehealthcheck", m.Name, "namespace", m.Namespace)
	logger = logger.WithValues("cluster", cluster.Name)

	// Create client for target cluster
	clusterClient, err := remote.NewClusterClient(ctx, r.Client, util.ObjectKey(cluster), r.scheme)
	if err != nil {
		logger.Error(err, "Error building target cluster client")
		return ctrl.Result{}, err
	}

	if err := r.watchClusterNodes(ctx, r.Client, cluster); err != nil {
		logger.Error(err, "Error watching nodes on target cluster")
		return ctrl.Result{}, err
	}

	// fetch all targets
	logger.V(3).Info("Finding targets")
	targets, err := r.getTargetsFromMHC(clusterClient, cluster, m)
	if err != nil {
		logger.Error(err, "Failed to fetch targets from MachineHealthCheck")
		return ctrl.Result{}, err
	}
	totalTargets := len(targets)
	m.Status.ExpectedMachines = int32(totalTargets)

	// health check all targets and reconcile mhc status
	currentHealthy, needRemediationTargets, nextCheckTimes := r.healthCheckTargets(targets, logger, m.Spec.NodeStartupTimeout.Duration)
	m.Status.CurrentHealthy = int32(currentHealthy)

	// check MHC current health against MaxUnhealthy
	if !isAllowedRemediation(m) {
		logger.V(3).Info(
			"Short-circuiting remediation",
			"total target", totalTargets,
			"max unhealthy", m.Spec.MaxUnhealthy,
			"unhealthy targets", totalTargets-currentHealthy,
		)

		r.recorder.Eventf(
			m,
			corev1.EventTypeWarning,
			EventRemediationRestricted,
			"Remediation restricted due to exceeded number of unhealthy machines (total: %v, unhealthy: %v, maxUnhealthy: %v)",
			totalTargets,
			totalTargets-currentHealthy,
			m.Spec.MaxUnhealthy,
		)
		return reconcile.Result{Requeue: true}, nil
	}
	logger.V(3).Info(
		"Remediations are allowed",
		"total target", totalTargets,
		"max unhealthy", m.Spec.MaxUnhealthy,
		"unhealthy targets", totalTargets-currentHealthy,
	)

	// remediate
	errList := []error{}
	for _, t := range needRemediationTargets {
		logger.V(3).Info("Target meets unhealthy criteria, triggers remediation", "target", t.string())
		if err := t.remediate(ctx, logger, r.Client, r.recorder); err != nil {
			logger.Error(err, "Error remediating target", "target", t.string())
			errList = append(errList, err)
		}
	}

	// handle remediation errors
	if len(errList) > 0 {
		logger.V(3).Info("Error(s) remediating request, requeueing")
		return reconcile.Result{}, kerrors.NewAggregate(errList)
	}

	if minNextCheck := minDuration(nextCheckTimes); minNextCheck > 0 {
		logger.V(3).Info("Some targets might go unhealthy. Ensuring a requeue happens", "requeueIn", minNextCheck.Truncate(time.Second).String())
		return ctrl.Result{RequeueAfter: minNextCheck}, nil
	}

	logger.V(3).Info("No more targets meet unhealthy criteria")

	return ctrl.Result{}, nil
}

func (r *MachineHealthCheckReconciler) indexMachineHealthCheckByClusterName(object runtime.Object) []string {
	mhc, ok := object.(*clusterv1.MachineHealthCheck)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a MachineHealthCheck", "type", fmt.Sprintf("%T", object))
		return nil
	}

	return []string{mhc.Spec.ClusterName}
}

// clusterToMachineHealthCheck maps events from Cluster objects to
// MachineHealthCheck objects that belong to the Cluster
func (r *MachineHealthCheckReconciler) clusterToMachineHealthCheck(o handler.MapObject) []reconcile.Request {
	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a Cluster", "type", fmt.Sprintf("%T", o))
		return nil
	}

	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := r.Client.List(
		context.TODO(),
		mhcList,
		client.InNamespace(c.Namespace),
		client.MatchingFields{mhcClusterNameIndex: c.Name},
	); err != nil {
		r.Log.Error(err, "Unable to list MachineHealthChecks", "cluster", c.Name, "namespace", c.Namespace)
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
// MachineHealthCheck objects that monitor the given machine
func (r *MachineHealthCheckReconciler) machineToMachineHealthCheck(o handler.MapObject) []reconcile.Request {
	m, ok := o.Object.(*clusterv1.Machine)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a Machine", "type", fmt.Sprintf("%T", o))
		return nil
	}

	mhcList := &clusterv1.MachineHealthCheckList{}
	if err := r.Client.List(
		context.Background(),
		mhcList,
		&client.ListOptions{Namespace: m.Namespace},
		client.MatchingFields{mhcClusterNameIndex: m.Spec.ClusterName},
	); err != nil {
		r.Log.Error(err, "Unable to list MachineHealthChecks", "machine", m.Name, "namespace", m.Namespace)
		return nil
	}

	var requests []reconcile.Request
	for k := range mhcList.Items {
		mhc := &mhcList.Items[k]
		if hasMatchingLabels(mhc.Spec.Selector, m.Labels) {
			key := util.ObjectKey(mhc)
			requests = append(requests, reconcile.Request{NamespacedName: key})
		}
	}
	return requests
}

func (r *MachineHealthCheckReconciler) nodeToMachineHealthCheck(o handler.MapObject) []reconcile.Request {
	node, ok := o.Object.(*corev1.Node)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a Node", "type", fmt.Sprintf("%T", o))
		return nil
	}

	machine, err := r.getMachineFromNode(node.Name)
	if machine == nil || err != nil {
		r.Log.Error(err, "Unable to retrieve machine from node", "node", node.GetName())
		return nil
	}

	return r.machineToMachineHealthCheck(handler.MapObject{Object: machine})
}

func (r *MachineHealthCheckReconciler) getMachineFromNode(nodeName string) (*clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(
		context.TODO(),
		machineList,
		client.MatchingFields{machineNodeNameIndex: nodeName},
	); err != nil {
		return nil, errors.Wrap(err, "failed getting machine list")
	}
	if len(machineList.Items) != 1 {
		return nil, errors.Errorf("expecting one machine for node %v, got: %v", nodeName, machineList.Items)
	}
	return &machineList.Items[0], nil
}

func (r *MachineHealthCheckReconciler) watchClusterNodes(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) error {
	key := util.ObjectKey(cluster)
	if _, ok := r.getClusterCache(key); ok {
		// watch was already set up for this cluster
		return nil
	}

	return r.createClusterCache(ctx, c, key)
}

func (r *MachineHealthCheckReconciler) getClusterCache(key client.ObjectKey) (cache.Cache, bool) {
	r.clusterCachesLock.RLock()
	defer r.clusterCachesLock.RUnlock()

	c, ok := r.clusterCaches[key]
	return c, ok
}

func (r *MachineHealthCheckReconciler) createClusterCache(ctx context.Context, c client.Client, key client.ObjectKey) error {
	r.clusterCachesLock.Lock()
	defer r.clusterCachesLock.Unlock()

	// Double check the key still doesn't exist under write lock
	if _, ok := r.clusterCaches[key]; ok {
		// An informer was created while waiting for the lock
		return nil
	}

	config, err := remote.RESTConfig(ctx, c, key)
	if err != nil {
		return errors.Wrap(err, "error fetching remote cluster config")
	}

	clusterCache, err := cache.New(config, cache.Options{})
	if err != nil {
		return errors.Wrap(err, "error creating cache for remote cluster")
	}
	go clusterCache.Start(ctx.Done())

	err = r.controller.Watch(
		source.NewKindWithCache(&corev1.Node{}, clusterCache),
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.nodeToMachineHealthCheck)},
	)
	if err != nil {
		return errors.Wrap(err, "error watching nodes on target cluster")
	}

	r.clusterCaches[key] = clusterCache
	return nil
}

func (r *MachineHealthCheckReconciler) indexMachineByNodeName(object runtime.Object) []string {
	machine, ok := object.(*clusterv1.Machine)
	if !ok {
		r.Log.Error(errors.New("incorrect type"), "expected a Machine", "type", fmt.Sprintf("%T", object))
		return nil
	}

	if machine.Status.NodeRef != nil {
		return []string{machine.Status.NodeRef.Name}
	}

	return nil
}

// isAllowedRemediation checks the value of the MaxUnhealthy field to determine
// whether remediation should be allowed or not
func isAllowedRemediation(mhc *clusterv1.MachineHealthCheck) bool {
	if mhc.Spec.MaxUnhealthy == nil {
		return true
	}
	maxUnhealthy, err := intstr.GetValueFromIntOrPercent(mhc.Spec.MaxUnhealthy, int(mhc.Status.ExpectedMachines), false)
	if err != nil {
		return false
	}

	// If unhealthy is above maxUnhealthy, short circuit any further remediation
	unhealthy := mhc.Status.ExpectedMachines - mhc.Status.CurrentHealthy
	return int(unhealthy) <= maxUnhealthy
}
