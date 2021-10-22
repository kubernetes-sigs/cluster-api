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

package topology

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/util/annotations"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// ClusterClassReconciler reconciles the ClusterClass object.
type ClusterClassReconciler struct {
	Client client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// UnstructuredCachingClient provides a client that forces caching of unstructured objects,
	// thus allowing to optimize reads for templates or provider specific objects.
	UnstructuredCachingClient client.Client
}

func (r *ClusterClassReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		Named("topology/clusterclass").
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

func (r *ClusterClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
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
	if annotations.HasPausedAnnotation(clusterClass) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	if !clusterClass.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// We use the patchHelper to patch potential changes to the ObjectReferences in ClusterClass.
	patchHelper, err := patch.NewHelper(clusterClass, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, clusterClass); err != nil {
			reterr = kerrors.NewAggregate([]error{
				reterr,
				errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: clusterClass})},
			)
		}
	}()

	return r.reconcile(ctx, clusterClass)
}

func (r *ClusterClassReconciler) reconcile(ctx context.Context, clusterClass *clusterv1.ClusterClass) (ctrl.Result, error) {
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

	// Ensure all the referenced objects are owned by the ClusterClass and that references are
	// upgraded to the latest contract.
	// Nb. Some external objects can be referenced multiple times in the ClusterClass. We
	// update the API contracts of all the references but we set the owner reference on the unique
	// external object only once.
	errs := []error{}
	patchedRefs := sets.NewString()
	for i := range refs {
		ref := refs[i]
		uniqueKey := uniqueObjectRefKey(ref)
		if err := r.reconcileExternal(ctx, clusterClass, ref, !patchedRefs.Has(uniqueKey)); err != nil {
			errs = append(errs, err)
			continue
		}
		patchedRefs.Insert(uniqueKey)
	}

	return ctrl.Result{}, kerrors.NewAggregate(errs)
}

func (r *ClusterClassReconciler) reconcileExternal(ctx context.Context, clusterClass *clusterv1.ClusterClass, ref *corev1.ObjectReference, setOwnerRef bool) error {
	log := ctrl.LoggerFrom(ctx)

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return errors.Wrapf(err, "failed to update reference API contract of %s", tlog.KRef{Ref: ref})
	}

	// If we dont need to set the ownerReference then return early.
	if !setOwnerRef {
		return nil
	}

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, clusterClass.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return errors.Wrapf(err, "Could not find external object for the cluster class. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
		}
		return errors.Wrapf(err, "failed to get the external object for the cluster class. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
	}

	// If external ref is paused, return early.
	if annotations.HasPausedAnnotation(obj) {
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
