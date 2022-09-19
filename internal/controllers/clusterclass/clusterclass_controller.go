/*
Copyright 2021 The Kubernetes Authors.

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

package clusterclass

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles the ClusterClass object.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// UnstructuredCachingClient provides a client that forces caching of unstructured objects,
	// thus allowing to optimize reads for templates or provider specific objects.
	UnstructuredCachingClient client.Client
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		Named("clusterclass").
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	clusterClass := &clusterv1.ClusterClass{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterClass); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Error reading the object  - requeue the request.
		return ctrl.Result{}, err
	}

	// Return early if the ClusterClass is paused.
	if annotations.HasPaused(clusterClass) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	if !clusterClass.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, clusterClass)
}

func (r *Reconciler) reconcile(ctx context.Context, clusterClass *clusterv1.ClusterClass) (ctrl.Result, error) {
	// Collect all the reference from the ClusterClass to templates.
	refs := []*corev1.ObjectReference{}

	if clusterClass.Spec.Infrastructure.Ref != nil {
		refs = append(refs, clusterClass.Spec.Infrastructure.Ref)
	}

	if clusterClass.Spec.ControlPlane.Ref != nil {
		refs = append(refs, clusterClass.Spec.ControlPlane.Ref)
	}
	if clusterClass.Spec.ControlPlane.MachineInfrastructure != nil && clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		refs = append(refs, clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)
	}

	for _, mdClass := range clusterClass.Spec.Workers.MachineDeployments {
		if mdClass.Template.Bootstrap.Ref != nil {
			refs = append(refs, mdClass.Template.Bootstrap.Ref)
		}
		if mdClass.Template.Infrastructure.Ref != nil {
			refs = append(refs, mdClass.Template.Infrastructure.Ref)
		}
	}

	// Ensure all referenced objects are owned by the ClusterClass.
	// Nb. Some external objects can be referenced multiple times in the ClusterClass,
	// but we only want to set the owner reference once per unique external object.
	// For example the same KubeadmConfigTemplate could be referenced in multiple MachineDeployment
	// classes.
	errs := []error{}
	reconciledRefs := sets.NewString()
	for i := range refs {
		ref := refs[i]
		uniqueKey := uniqueObjectRefKey(ref)

		// Continue as we only have to reconcile every referenced object once.
		if reconciledRefs.Has(uniqueKey) {
			continue
		}

		if err := r.reconcileExternal(ctx, clusterClass, ref); err != nil {
			errs = append(errs, err)
			continue
		}
		reconciledRefs.Insert(uniqueKey)
	}

	return ctrl.Result{}, kerrors.NewAggregate(errs)
}

func (r *Reconciler) reconcileExternal(ctx context.Context, clusterClass *clusterv1.ClusterClass, ref *corev1.ObjectReference) error {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, clusterClass.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return errors.Wrapf(err, "Could not find external object for the cluster class. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
		}
		return errors.Wrapf(err, "failed to get the external object for the cluster class. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
	}

	// If referenced object is paused, return early.
	if annotations.HasPaused(obj) {
		log.V(3).Info("External object referenced is paused", "refGroupVersionKind", ref.GroupVersionKind(), "refName", ref.Name)
		return nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: obj})
	}

	// Set external object ControllerReference to the ClusterClass.
	if err := controllerutil.SetOwnerReference(clusterClass, obj, r.Client.Scheme()); err != nil {
		return errors.Wrapf(err, "failed to set cluster class owner reference for %s", tlog.KObj{Obj: obj})
	}

	// Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return errors.Wrapf(err, "failed to patch object %s", tlog.KObj{Obj: obj})
	}

	return nil
}

func uniqueObjectRefKey(ref *corev1.ObjectReference) string {
	return fmt.Sprintf("Name:%s, Namespace:%s, Kind:%s, APIVersion:%s", ref.Name, ref.Namespace, ref.Kind, ref.APIVersion)
}
