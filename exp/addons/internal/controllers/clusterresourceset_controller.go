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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	resourcepredicates "sigs.k8s.io/cluster-api/exp/addons/internal/controllers/predicates"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// ErrSecretTypeNotSupported signals that a Secret is not supported.
var ErrSecretTypeNotSupported = errors.New("unsupported secret type")

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=clusterresourcesets/status,verbs=get;update;patch

// ClusterResourceSetReconciler reconciles a ClusterResourceSet object.
type ClusterResourceSetReconciler struct {
	Client  client.Client
	Tracker *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterResourceSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options, partialSecretCache cache.Cache) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1.ClusterResourceSet{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.clusterToClusterResourceSet),
		).
		WatchesMetadata(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(
				resourceToClusterResourceSetFunc[client.Object](r.Client),
			),
			builder.WithPredicates(
				resourcepredicates.TypedResourceCreateOrUpdate[client.Object](ctrl.LoggerFrom(ctx)),
			),
		).
		WatchesRawSource(source.Kind(
			partialSecretCache,
			&metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
			},
			handler.TypedEnqueueRequestsFromMapFunc(
				resourceToClusterResourceSetFunc[*metav1.PartialObjectMetadata](r.Client),
			),
			resourcepredicates.TypedResourceCreateOrUpdate[*metav1.PartialObjectMetadata](ctrl.LoggerFrom(ctx)),
		)).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

func (r *ClusterResourceSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the ClusterResourceSet instance.
	clusterResourceSet := &addonsv1.ClusterResourceSet{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterResourceSet); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(clusterResourceSet, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to patch the object and status after each reconciliation.
		// Patch ObservedGeneration only if the reconciliation completed successfully.
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, clusterResourceSet, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	clusters, err := r.getClustersByClusterResourceSetSelector(ctx, clusterResourceSet)
	if err != nil {
		log.Error(err, "Failed fetching clusters that matches ClusterResourceSet labels", "ClusterResourceSet", klog.KObj(clusterResourceSet))
		conditions.MarkFalse(clusterResourceSet, addonsv1.ResourcesAppliedCondition, addonsv1.ClusterMatchFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	// Handle deletion reconciliation loop.
	if !clusterResourceSet.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, clusters, clusterResourceSet)
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(clusterResourceSet, addonsv1.ClusterResourceSetFinalizer) {
		controllerutil.AddFinalizer(clusterResourceSet, addonsv1.ClusterResourceSetFinalizer)
		return ctrl.Result{}, nil
	}

	errs := []error{}
	errClusterLockedOccurred := false
	for _, cluster := range clusters {
		if err := r.ApplyClusterResourceSet(ctx, cluster, clusterResourceSet); err != nil {
			// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
			// the current cluster because of concurrent access.
			if errors.Is(err, remote.ErrClusterLocked) {
				log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
				errClusterLockedOccurred = true
			} else {
				// Append the error if the error is not ErrClusterLocked.
				errs = append(errs, err)
			}
		}
	}

	// Return an aggregated error if errors occurred.
	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	// Requeue if ErrClusterLocked was returned for one of the clusters.
	if errClusterLockedOccurred {
		// Requeue after a minute to not end up in exponential delayed requeue which
		// could take up to 16m40s.
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete removes the deleted ClusterResourceSet from all the ClusterResourceSetBindings it is added to.
func (r *ClusterResourceSetReconciler) reconcileDelete(ctx context.Context, clusters []*clusterv1.Cluster, crs *addonsv1.ClusterResourceSet) error {
	for _, cluster := range clusters {
		log := ctrl.LoggerFrom(ctx, "Cluster", klog.KObj(cluster))

		clusterResourceSetBinding := &addonsv1.ClusterResourceSetBinding{}
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		}
		if err := r.Client.Get(ctx, clusterResourceSetBindingKey, clusterResourceSetBinding); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get ClusterResourceSetBinding during ClusterResourceSet deletion")
			}
			controllerutil.RemoveFinalizer(crs, addonsv1.ClusterResourceSetFinalizer)
			return nil
		}

		// Initialize the patch helper.
		patchHelper, err := patch.NewHelper(clusterResourceSetBinding, r.Client)
		if err != nil {
			return err
		}

		clusterResourceSetBinding.RemoveBinding(crs)
		clusterResourceSetBinding.OwnerReferences = util.RemoveOwnerRef(clusterResourceSetBinding.GetOwnerReferences(), metav1.OwnerReference{
			APIVersion: addonsv1.GroupVersion.String(),
			Kind:       "ClusterResourceSet",
			Name:       crs.Name,
		})

		// If CRS list is empty in the binding, delete the binding else
		// attempt to Patch the ClusterResourceSetBinding object after delete reconciliation if there is at least 1 binding left.
		if len(clusterResourceSetBinding.Spec.Bindings) == 0 {
			if r.Client.Delete(ctx, clusterResourceSetBinding) != nil {
				log.Error(err, "Failed to delete empty ClusterResourceSetBinding")
			}
		} else if err := patchHelper.Patch(ctx, clusterResourceSetBinding); err != nil {
			return err
		}
	}

	controllerutil.RemoveFinalizer(crs, addonsv1.ClusterResourceSetFinalizer)
	return nil
}

// getClustersByClusterResourceSetSelector fetches Clusters matched by the ClusterResourceSet's label selector that are in the same namespace as the ClusterResourceSet object.
func (r *ClusterResourceSetReconciler) getClustersByClusterResourceSetSelector(ctx context.Context, clusterResourceSet *addonsv1.ClusterResourceSet) ([]*clusterv1.Cluster, error) {
	log := ctrl.LoggerFrom(ctx)

	clusterList := &clusterv1.ClusterList{}
	selector, err := metav1.LabelSelectorAsSelector(&clusterResourceSet.Spec.ClusterSelector)
	if err != nil {
		return nil, errors.Wrap(err, "unable to convert selector")
	}

	// If a ClusterResourceSet has a nil or empty selector, it should match nothing, not everything.
	if selector.Empty() {
		log.Info("Empty ClusterResourceSet selector: No clusters are selected.")
		return nil, nil
	}

	if err := r.Client.List(ctx, clusterList, client.InNamespace(clusterResourceSet.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, errors.Wrap(err, "failed to list clusters")
	}

	clusters := []*clusterv1.Cluster{}
	for i := range clusterList.Items {
		c := &clusterList.Items[i]
		if c.DeletionTimestamp.IsZero() {
			clusters = append(clusters, c)
		}
	}
	return clusters, nil
}

// ApplyClusterResourceSet applies resources in a ClusterResourceSet to a Cluster. Once applied, a record will be added to the
// cluster's ClusterResourceSetBinding.
// In ApplyOnce strategy, resources are applied only once to a particular cluster. ClusterResourceSetBinding is used to check if a resource is applied before.
// It applies resources best effort and continue on scenarios like: unsupported resource types, failure during creation, missing resources.
// In Reconcile strategy, resources are re-applied to a particular cluster when their definition changes. The hash in ClusterResourceSetBinding is used to check
// if a resource has changed or not.
// TODO: If a resource already exists in the cluster but not applied by ClusterResourceSet, the resource will be updated ?
func (r *ClusterResourceSetReconciler) ApplyClusterResourceSet(ctx context.Context, cluster *clusterv1.Cluster, clusterResourceSet *addonsv1.ClusterResourceSet) error {
	log := ctrl.LoggerFrom(ctx, "Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		conditions.MarkFalse(clusterResourceSet, addonsv1.ResourcesAppliedCondition, addonsv1.RemoteClusterClientFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return err
	}

	// Ensure that the Kubernetes API Server service has been created in the remote cluster before applying the ClusterResourceSet to avoid service IP conflict.
	// This action is required when the remote cluster Kubernetes version is lower than v1.25.
	// TODO: Remove this action once CAPI no longer supports Kubernetes versions below v1.25. See: https://github.com/kubernetes-sigs/cluster-api/issues/7804
	if err = ensureKubernetesServiceCreated(ctx, remoteClient); err != nil {
		return errors.Wrapf(err, "failed to retrieve the Service for Kubernetes API Server of the cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	// Get ClusterResourceSetBinding object for the cluster.
	clusterResourceSetBinding, err := r.getOrCreateClusterResourceSetBinding(ctx, cluster, clusterResourceSet)
	if err != nil {
		return err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(clusterResourceSetBinding, r.Client)
	if err != nil {
		return err
	}

	defer func() {
		// Always attempt to Patch the ClusterResourceSetBinding object after each reconciliation.
		if err := patchHelper.Patch(ctx, clusterResourceSetBinding); err != nil {
			log.Error(err, "Failed to patch config")
		}
	}()

	// Ensure that the owner references are set on the ClusterResourceSetBinding.
	clusterResourceSetBinding.SetOwnerReferences(util.EnsureOwnerRef(clusterResourceSetBinding.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: addonsv1.GroupVersion.String(),
		Kind:       "ClusterResourceSet",
		Name:       clusterResourceSet.Name,
		UID:        clusterResourceSet.UID,
	}))
	errList := []error{}
	resourceSetBinding := clusterResourceSetBinding.GetOrCreateBinding(clusterResourceSet)

	// Iterate all resources and apply them to the cluster and update the resource status in the ClusterResourceSetBinding object.
	for _, resource := range clusterResourceSet.Spec.Resources {
		unstructuredObj, err := r.getResource(ctx, resource, cluster.GetNamespace())
		if err != nil {
			if err == ErrSecretTypeNotSupported {
				conditions.MarkFalse(clusterResourceSet, addonsv1.ResourcesAppliedCondition, addonsv1.WrongSecretTypeReason, clusterv1.ConditionSeverityWarning, err.Error())
			} else {
				conditions.MarkFalse(clusterResourceSet, addonsv1.ResourcesAppliedCondition, addonsv1.RetrievingResourceFailedReason, clusterv1.ConditionSeverityWarning, err.Error())

				// Continue without adding the error to the aggregate if we can't find the resource.
				if apierrors.IsNotFound(err) {
					continue
				}
			}
			errList = append(errList, err)
			continue
		}

		// Ensure an ownerReference to the clusterResourceSet is on the resource.
		if err := r.ensureResourceOwnerRef(ctx, clusterResourceSet, unstructuredObj); err != nil {
			log.Error(err, "Failed to add ClusterResourceSet as resource owner reference",
				"Resource type", unstructuredObj.GetKind(), "Resource name", unstructuredObj.GetName())
			errList = append(errList, err)
		}

		resourceScope, err := reconcileScopeForResource(clusterResourceSet, resource, resourceSetBinding, unstructuredObj)
		if err != nil {
			resourceSetBinding.SetBinding(addonsv1.ResourceBinding{
				ResourceRef:     resource,
				Hash:            "",
				Applied:         false,
				LastAppliedTime: &metav1.Time{Time: time.Now().UTC()},
			})

			errList = append(errList, err)
			continue
		}

		if !resourceScope.needsApply() {
			continue
		}

		// Set status in ClusterResourceSetBinding in case of early continue due to a failure.
		// Set only when resource is retrieved successfully.
		resourceSetBinding.SetBinding(addonsv1.ResourceBinding{
			ResourceRef:     resource,
			Hash:            "",
			Applied:         false,
			LastAppliedTime: &metav1.Time{Time: time.Now().UTC()},
		})

		// Apply all values in the key-value pair of the resource to the cluster.
		// As there can be multiple key-value pairs in a resource, each value may have multiple objects in it.
		isSuccessful := true
		if err := resourceScope.apply(ctx, remoteClient); err != nil {
			isSuccessful = false
			log.Error(err, "Failed to apply ClusterResourceSet resource", resource.Kind, klog.KRef(clusterResourceSet.Namespace, resource.Name))
			conditions.MarkFalse(clusterResourceSet, addonsv1.ResourcesAppliedCondition, addonsv1.ApplyFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
			errList = append(errList, err)
		}

		resourceSetBinding.SetBinding(addonsv1.ResourceBinding{
			ResourceRef:     resource,
			Hash:            resourceScope.hash(),
			Applied:         isSuccessful,
			LastAppliedTime: &metav1.Time{Time: time.Now().UTC()},
		})
	}
	if len(errList) > 0 {
		return kerrors.NewAggregate(errList)
	}

	conditions.MarkTrue(clusterResourceSet, addonsv1.ResourcesAppliedCondition)

	return nil
}

// getResource retrieves the requested resource and convert it to unstructured type.
// Unsupported resource kinds are not denied by validation webhook, hence no need to check here.
// Only supports Secrets/Configmaps as resource types and allow using resources in the same namespace with the cluster.
func (r *ClusterResourceSetReconciler) getResource(ctx context.Context, resourceRef addonsv1.ResourceRef, namespace string) (*unstructured.Unstructured, error) {
	resourceName := types.NamespacedName{Name: resourceRef.Name, Namespace: namespace}

	var resourceInterface interface{}
	switch resourceRef.Kind {
	case string(addonsv1.ConfigMapClusterResourceSetResourceKind):
		resourceConfigMap, err := getConfigMap(ctx, r.Client, resourceName)
		if err != nil {
			return nil, err
		}

		resourceInterface = resourceConfigMap.DeepCopyObject()
	case string(addonsv1.SecretClusterResourceSetResourceKind):
		resourceSecret, err := getSecret(ctx, r.Client, resourceName)
		if err != nil {
			return nil, err
		}

		if resourceSecret.Type != addonsv1.ClusterResourceSetSecretType {
			return nil, ErrSecretTypeNotSupported
		}
		resourceInterface = resourceSecret.DeepCopyObject()
	}

	raw := &unstructured.Unstructured{}
	err := r.Client.Scheme().Convert(resourceInterface, raw, nil)
	if err != nil {
		return nil, err
	}

	return raw, nil
}

// ensureResourceOwnerRef adds the ClusterResourceSet as a OwnerReference to the resource.
func (r *ClusterResourceSetReconciler) ensureResourceOwnerRef(ctx context.Context, clusterResourceSet *addonsv1.ClusterResourceSet, resource *unstructured.Unstructured) error {
	obj := resource.DeepCopy()
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}
	newRef := metav1.OwnerReference{
		APIVersion: addonsv1.GroupVersion.String(),
		Kind:       clusterResourceSet.GroupVersionKind().Kind,
		Name:       clusterResourceSet.GetName(),
		UID:        clusterResourceSet.GetUID(),
	}
	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), newRef))
	return patchHelper.Patch(ctx, obj)
}

// clusterToClusterResourceSet is mapper function that maps clusters to ClusterResourceSet.
func (r *ClusterResourceSetReconciler) clusterToClusterResourceSet(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}

	cluster, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	resourceList := &addonsv1.ClusterResourceSetList{}
	if err := r.Client.List(ctx, resourceList, client.InNamespace(cluster.Namespace)); err != nil {
		return nil
	}

	labels := labels.Set(cluster.GetLabels())
	for i := range resourceList.Items {
		rs := &resourceList.Items[i]

		selector, err := metav1.LabelSelectorAsSelector(&rs.Spec.ClusterSelector)
		if err != nil {
			return nil
		}

		// If a ClusterResourceSet has a nil or empty selector, it should match nothing, not everything.
		if selector.Empty() {
			return nil
		}

		if !selector.Matches(labels) {
			continue
		}

		name := client.ObjectKey{Namespace: rs.Namespace, Name: rs.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}
	return result
}

// resourceToClusterResourceSetFunc returns a typed mapper function that maps resources to ClusterResourceSet.
func resourceToClusterResourceSetFunc[T client.Object](ctrlClient client.Client) handler.TypedMapFunc[T] {
	return func(ctx context.Context, o T) []ctrl.Request {
		result := []ctrl.Request{}

		// Add all ClusterResourceSet owners.
		for _, owner := range o.GetOwnerReferences() {
			if owner.Kind == "ClusterResourceSet" {
				name := client.ObjectKey{Namespace: o.GetNamespace(), Name: owner.Name}
				result = append(result, ctrl.Request{NamespacedName: name})
			}
		}

		// If there is any ClusterResourceSet owner, that means the resource is reconciled before,
		// and existing owners are the only matching ClusterResourceSets to this resource, so no need to return all ClusterResourceSets.
		if len(result) > 0 {
			return result
		}

		// Only core group is accepted as resources group
		if o.GetObjectKind().GroupVersionKind().Group != "" {
			return result
		}

		crsList := &addonsv1.ClusterResourceSetList{}
		if err := ctrlClient.List(ctx, crsList, client.InNamespace(o.GetNamespace())); err != nil {
			return nil
		}
		objKind, err := apiutil.GVKForObject(o, ctrlClient.Scheme())
		if err != nil {
			return nil
		}
		for _, crs := range crsList.Items {
			for _, resource := range crs.Spec.Resources {
				if resource.Kind == objKind.Kind && resource.Name == o.GetName() {
					name := client.ObjectKey{Namespace: o.GetNamespace(), Name: crs.Name}
					result = append(result, ctrl.Request{NamespacedName: name})
					break
				}
			}
		}

		return result
	}
}
