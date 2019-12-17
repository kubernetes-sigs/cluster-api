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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

type TemplateCloner interface {
	CloneTemplate(ctx context.Context, c client.Client, ref *corev1.ObjectReference, namespace, clusterName string, owner *metav1.OwnerReference) (*corev1.ObjectReference, error)
}

type KubeadmConfigGenerator interface {
	GenerateKubeadmConfig(ctx context.Context, c client.Client, namespace, namePrefix, clusterName string, spec *bootstrapv1.KubeadmConfigSpec, owner *metav1.OwnerReference) (*corev1.ObjectReference, error)
}

type MachineGenerator interface {
	GenerateMachine(ctx context.Context, c client.Client, namespace, namePrefix, clusterName, version string, infraRef, bootstrapRef *corev1.ObjectReference, labels map[string]string, owner *metav1.OwnerReference) error
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes;kubeadmcontrolplanes/status,verbs=get;list;watch;create;update;patch;delete

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object
type KubeadmControlPlaneReconciler struct {
	Client client.Client
	Log    logr.Logger

	TemplateCloner         TemplateCloner
	KubeadmConfigGenerator KubeadmConfigGenerator
	MachineGenerator       MachineGenerator

	controller controller.Controller
	recorder   record.EventRecorder
}

func (r *KubeadmControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlane{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.ClusterToKubeadmControlPlane)},
		).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("kubeadm-control-plane-controller")

	return nil
}

func (r *KubeadmControlPlaneReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, _ error) {
	logger := r.Log.WithValues("kubeadmControlPlane", req.Name, "namespace", req.Namespace)
	ctx := context.Background()

	// Fetch the KubeadmControlPlane instance.
	kubeadmControlPlane := &controlplanev1.KubeadmControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kubeadmControlPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to retrieve requested KubeadmControlPlane resource from the API Server")
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kubeadmControlPlane, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		// Always attempt to Patch the KubeadmControlPlane object and status after each reconciliation.
		if patchErr := patchHelper.Patch(ctx, kubeadmControlPlane); patchErr != nil {
			logger.Error(patchErr, "Failed to patch KubeadmControlPlane")
			res.Requeue = true
		}
	}()

	// Handle deletion reconciliation loop.
	if !kubeadmControlPlane.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, kubeadmControlPlane, logger), nil
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, kubeadmControlPlane, logger), nil
}

// reconcile handles KubeadmControlPlane reconciliation.
func (r *KubeadmControlPlaneReconciler) reconcile(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, logger logr.Logger) ctrl.Result {
	// If object doesn't have a finalizer, add one.
	controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{Requeue: true}
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{}
	}
	logger = logger.WithValues("cluster", cluster.Name)

	labelSelector := generateKubeadmControlPlaneSelector(cluster.Name)
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		// Since we are building up the LabelSelector above, this should not fail
		logger.Error(err, "failed to parse label selector")
		return ctrl.Result{Requeue: true}
	}
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	kcp.Status.Selector = selector.String()

	selectorMap, err := metav1.LabelSelectorAsMap(labelSelector)
	if err != nil {
		// Since we are building up the LabelSelector above, this should not fail
		logger.Error(err, "failed to convert label selector to a map")
		return ctrl.Result{Requeue: true}
	}

	// Get all Machines linked to this KubeadmControlPlane.
	allMachines := &clusterv1.MachineList{}
	err = r.Client.List(
		context.Background(),
		allMachines,
		client.InNamespace(kcp.Namespace),
		client.MatchingLabels(selectorMap),
	)
	if err != nil {
		logger.Error(err, "failed to list machines")
		return ctrl.Result{Requeue: true}
	}

	// Filter out any machines that are not owned by this controller
	// TODO: handle proper adoption of Machines
	var ownedMachines []*clusterv1.Machine
	for i := range allMachines.Items {
		m := allMachines.Items[i]
		controllerRef := metav1.GetControllerOf(&m)
		if controllerRef != nil && controllerRef.Kind == "KubeadmControlPlane" && controllerRef.Name == kcp.Name {
			ownedMachines = append(ownedMachines, &m)
		}
	}

	// Generate Cluster Certificates if needed
	config := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	config.JoinConfiguration = nil
	if config.ClusterConfiguration == nil {
		config.ClusterConfiguration = &kubeadmv1.ClusterConfiguration{}
	}
	certificates := secret.NewCertificatesForInitialControlPlane(config.ClusterConfiguration)
	err = certificates.LookupOrGenerate(
		ctx,
		r.Client,
		types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name},
		*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
	)
	if err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		return ctrl.Result{Requeue: true}
	}

	// If ControlPlaneEndpoint is not set, return early
	if cluster.Spec.ControlPlaneEndpoint.IsZero() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return reconcile.Result{}
	}

	// Generate Cluster Kubeconfig if needed
	err = r.reconcileKubeconfig(
		ctx,
		types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name},
		cluster.Spec.ControlPlaneEndpoint,
		kcp,
	)
	if err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			logger.Error(err, "required certificates not found, requeueing")
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: requeueErr.GetRequeueAfter(),
			}
		}
		logger.Error(err, "failed to reconcile Kubeconfig")
		return ctrl.Result{Requeue: true}
	}

	// Currently we are not handling upgrade, so treat all owned machines as one for now.
	// Once we start handling upgrade, we'll need to filter this list and act appropriately
	numMachines := len(ownedMachines)
	desiredReplicas := int(*kcp.Spec.Replicas)
	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// create new Machine w/ init
		logger.Info("Scaling to 1", "Desired Replicas", desiredReplicas, "Existing Replicas", numMachines)
		if initErr := r.initializeControlPlane(ctx, cluster, kcp, logger); initErr != nil {
			logger.Error(initErr, "Failed to initialize the Control Plane")
			return ctrl.Result{Requeue: true}
		}
	// scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		logger.Info("Scaling up", "Desired Replicas", desiredReplicas, "Existing Replicas", numMachines)
		// create a new Machine w/ join
		logger.Error(errors.New("Not Implemented"), "Should create new Machine using join here.")
	// scaling down
	case numMachines > desiredReplicas:
		logger.Info("Scaling down", "Desired Replicas", desiredReplicas, "Existing Replicas", numMachines)
		logger.Error(errors.New("Not Implemented"), "Should delete the appropriate Machine here.")
	}

	// TODO: handle updating of status, this should also likely be done even if other reconciliation steps fail
	logger.Error(errors.New("Not Implemented"), "Should update status here")
	return ctrl.Result{Requeue: true}
}

func (r *KubeadmControlPlaneReconciler) initializeControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, logger logr.Logger) error {
	ownerRef := metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))
	// Clone the infrastructure template
	infraRef, err := r.TemplateCloner.CloneTemplate(
		ctx,
		r.Client,
		&kcp.Spec.InfrastructureTemplate,
		kcp.Namespace,
		cluster.Name,
		ownerRef,
	)
	if err != nil {
		return errors.Wrap(err, "Failed to clone infrastructure template")
	}

	// Create the bootstrap configuration
	bootstrapSpec := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	bootstrapSpec.JoinConfiguration = nil
	bootstrapRef, err := r.KubeadmConfigGenerator.GenerateKubeadmConfig(
		ctx,
		r.Client,
		kcp.Namespace,
		kcp.Name,
		cluster.Name,
		bootstrapSpec,
		ownerRef,
	)
	if err != nil {
		return errors.Wrap(err, "Failed to generate bootstrap config")
	}

	// Create the Machine
	err = r.MachineGenerator.GenerateMachine(
		ctx,
		r.Client,
		kcp.Namespace,
		kcp.Name,
		cluster.Name,
		kcp.Spec.Version,
		infraRef,
		bootstrapRef,
		generateKubeadmControlPlaneLabels(cluster.Name),
		ownerRef,
	)
	if err != nil {
		logger.Error(err, "Unable to create Machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedCreateMachine", "Failed to create machine: %v", err)

		errs := []error{err}

		infraConfig := &unstructured.Unstructured{}
		infraConfig.SetKind(infraRef.Kind)
		infraConfig.SetAPIVersion(infraRef.APIVersion)
		infraConfig.SetNamespace(infraRef.Namespace)
		infraConfig.SetName(infraRef.Name)

		if err := r.Client.Delete(context.TODO(), infraConfig); !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup infrastructure configuration object after Machine creation error")
			errs = append(errs, err)
		}

		bootstrapConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bootstrapRef.Name,
				Namespace: bootstrapRef.Namespace,
			},
		}

		if err := r.Client.Delete(context.TODO(), bootstrapConfig); !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup bootstrap configuration object after Machine creation error")
			errs = append(errs, err)
		}

		return utilerrors.NewAggregate(errs)
	}

	return nil
}

func generateKubeadmControlPlaneSelector(clusterName string) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: generateKubeadmControlPlaneLabels(clusterName),
	}
}

func generateKubeadmControlPlaneLabels(clusterName string) map[string]string {
	return map[string]string{
		clusterv1.ClusterLabelName:             clusterName,
		clusterv1.MachineControlPlaneLabelName: "",
	}
}

// reconcileDelete handles KubeadmControlPlane deletion.
func (r *KubeadmControlPlaneReconciler) reconcileDelete(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, logger logr.Logger) ctrl.Result {
	err := errors.New("Not Implemented")

	if err != nil {
		logger.Error(err, "Not Implemented")
		return ctrl.Result{Requeue: true}
	}

	controllerutil.RemoveFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)
	return ctrl.Result{}
}

func (r *KubeadmControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, clusterName types.NamespacedName, endpoint clusterv1.APIEndpoint, kcp *controlplanev1.KubeadmControlPlane) error {
	if endpoint.IsZero() {
		return nil
	}

	_, err := secret.GetFromNamespacedName(r.Client, clusterName, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
		)
		if createErr != nil {
			if createErr == kubeconfig.ErrDependentCertificateNotFound {
				return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
					"could not find secret %q for Cluster %q in namespace %q, requeuing",
					secret.ClusterCA, clusterName.Name, clusterName.Namespace)
			}
			return createErr
		}
	case err != nil:
		return errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", clusterName.Name, clusterName.Namespace)
	}

	return nil
}

// ClusterToKubeadmControlPlane is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for KubeadmControlPlane based on updates to a Cluster.
func (r *KubeadmControlPlaneReconciler) ClusterToKubeadmControlPlane(o handler.MapObject) []ctrl.Request {
	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o.Object))
		return nil
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "KubeadmControlPlane" {
		name := client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}
		return []ctrl.Request{{NamespacedName: name}}
	}

	return nil
}
