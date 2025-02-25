/*
Copyright 2025 The Kubernetes Authors.

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

// Package crdmigrator contains the CRD migrator implementation.
package crdmigrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/contract"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// Phase is a phase of the CRD migration.
type Phase string

var (
	// AllPhase includes all phases, i.e. StorageVersionMigration and CleanupManagedFields.
	AllPhase Phase = "All"

	// StorageVersionMigrationPhase is the phase in which the storage version is migrated.
	// This means if the .status.storedVersions field of a CRD is not equal to [storageVersion],
	// a no-op patch is applied to all custom resources of the CRD to ensure they are all stored in
	// the current storageVersion in etcd. Afterward .status.storedVersions is set to [storageVersion].
	// For more information see:
	// https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
	StorageVersionMigrationPhase Phase = "StorageVersionMigration"

	// CleanupManagedFieldsPhase is the phase in which managedFields are cleaned up.
	// This means for all custom resources of a CRD, managedFields that contain apiVersions that
	// are not served anymore are removed.
	// For more information see:
	// https://github.com/kubernetes/kubernetes/issues/111937
	CleanupManagedFieldsPhase Phase = "CleanupManagedFields"
)

// CRDMigrator migrates CRDs.
type CRDMigrator struct {
	// Client is a cached client that is used:
	// * for all cached Get & List calls
	// * for all Patch calls
	Client client.Client

	// APIReader is a live client that is used:
	// * for all live Get & List calls
	APIReader client.Reader

	// Comma-separated list of CRD migration phases to skip.
	// Valid values are: All, StorageVersionMigration, CleanupManagedFields.
	SkipCRDMigrationPhases  string
	crdMigrationPhasesToRun sets.Set[Phase]

	// Config allows to configure which objects should be migrated.
	Config          map[client.Object]ByObjectConfig
	configByCRDName map[string]ByObjectConfig

	storageVersionMigrationCache cache.Cache[objectEntry]
}

// ByObjectConfig contains object-specific config for the CRD migration.
type ByObjectConfig struct {
	// UseCache configures if the cached client should be used for Get & List calls for this object.
	// Get & List calls on the cached client automatically trigger the creation of an informer which
	// requires a significant amount of memory.
	// Thus it is only recommended to enable this setting for objects for which the controller already uses the cache.
	// Note: If this is enabled, we will use the corresponding Go type of the object for Get & List calls to avoid
	// creating additional informers for UnstructuredList/PartialObjectMetadataList.
	UseCache bool
}

func (r *CRDMigrator) SetupWithManager(ctx context.Context, mgr ctrl.Manager, controllerOptions controller.Options) error {
	if err := r.setup(mgr.GetScheme()); err != nil {
		return err
	}

	if len(r.crdMigrationPhasesToRun) == 0 {
		// Nothing to do
		return nil
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "crdmigrator")
	err := ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{},
			// This controller uses a PartialObjectMetadata watch/informer to avoid an informer for CRDs
			// to reduce memory usage.
			// Core CAPI also already has an informer on PartialObjectMetadata for CRDs because it uses
			// conversion.UpdateReferenceAPIContract.
			builder.OnlyMetadata,
			builder.WithPredicates(
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
			),
		).
		Named("crdmigrator").
		WithOptions(controllerOptions).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

func (r *CRDMigrator) setup(scheme *runtime.Scheme) error {
	if r.Client == nil || r.APIReader == nil || len(r.Config) == 0 {
		return errors.New("Client and APIReader must not be nil and Config must not be empty")
	}

	r.crdMigrationPhasesToRun = sets.Set[Phase]{}.Insert(StorageVersionMigrationPhase, CleanupManagedFieldsPhase)
	if r.SkipCRDMigrationPhases != "" {
		for _, skipPhase := range strings.Split(r.SkipCRDMigrationPhases, ",") {
			switch skipPhase {
			case string(AllPhase):
				r.crdMigrationPhasesToRun.Delete(StorageVersionMigrationPhase, CleanupManagedFieldsPhase)
			case string(StorageVersionMigrationPhase):
				r.crdMigrationPhasesToRun.Delete(StorageVersionMigrationPhase)
			case string(CleanupManagedFieldsPhase):
				r.crdMigrationPhasesToRun.Delete(CleanupManagedFieldsPhase)
			default:
				return errors.Errorf("Invalid phase %s specified in SkipCRDMigrationPhases", skipPhase)
			}
		}
	}

	r.configByCRDName = map[string]ByObjectConfig{}
	for obj, cfg := range r.Config {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		if err != nil {
			return errors.Wrap(err, "failed to get GVK for object")
		}

		r.configByCRDName[contract.CalculateCRDName(gvk.Group, gvk.Kind)] = cfg
	}

	r.storageVersionMigrationCache = cache.New[objectEntry]()
	return nil
}

func (r *CRDMigrator) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	migrationConfig, ok := r.configByCRDName[req.Name]
	if !ok {
		// If there is no migrationConfig for this CRD, do nothing.
		return ctrl.Result{}, nil
	}

	crdPartial := &metav1.PartialObjectMetadata{}
	crdPartial.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
	if err := r.Client.Get(ctx, req.NamespacedName, crdPartial); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	currentGeneration := strconv.FormatInt(crdPartial.GetGeneration(), 10)
	if observedGeneration, ok := crdPartial.Annotations[clusterv1.CRDMigrationObservedGenerationAnnotation]; ok &&
		currentGeneration == observedGeneration {
		// If the current generation was already reconciled/observed, do nothing.
		return ctrl.Result{}, nil
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.APIReader.Get(ctx, req.NamespacedName, crd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Note: The CRD might have a newer generation than PartialObjectMeta above, we should use the
	// generation of the CRD we actually use for reconciliation.
	currentGeneration = strconv.FormatInt(crd.GetGeneration(), 10)

	storageVersion, err := storageVersionForCRD(crd)
	if err != nil {
		return ctrl.Result{}, err
	}

	originalCRD := crd.DeepCopy()
	defer func() {
		if reterr == nil {
			if crd.Annotations == nil {
				crd.Annotations = map[string]string{}
			}
			crd.Annotations[clusterv1.CRDMigrationObservedGenerationAnnotation] = currentGeneration
			if err := r.Client.Patch(ctx, crd, client.MergeFrom(originalCRD)); err != nil {
				reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to patch CustomResourceDefinition %s", crd.Name)})
			}
		}
	}()

	var customResourceObjects []client.Object
	if r.crdMigrationPhasesToRun.Has(StorageVersionMigrationPhase) && storageVersionMigrationRequired(crd, storageVersion) ||
		r.crdMigrationPhasesToRun.Has(CleanupManagedFieldsPhase) {
		// Get CustomResources only if we actually are going to run one of the phases.
		// Note: This relies on that the version that is storageVersion is also served.
		var err error
		customResourceObjects, err = r.listCustomResources(ctx, crd, migrationConfig, storageVersion)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// If phase should be run and .status.storedVersions != [storageVersion], run storage version migration.
	if r.crdMigrationPhasesToRun.Has(StorageVersionMigrationPhase) && storageVersionMigrationRequired(crd, storageVersion) {
		if err := r.reconcileStorageVersionMigration(ctx, crd, customResourceObjects, storageVersion); err != nil {
			return ctrl.Result{}, err
		}

		crd.Status.StoredVersions = []string{storageVersion}
		if err := r.Client.Status().Patch(ctx, crd, client.MergeFrom(originalCRD)); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to patch CustomResourceDefinition %s", crd.Name)
		}
	}

	if r.crdMigrationPhasesToRun.Has(CleanupManagedFieldsPhase) {
		if err := r.reconcileCleanupManagedFields(ctx, crd, customResourceObjects, migrationConfig, storageVersion); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func storageVersionForCRD(crd *apiextensionsv1.CustomResourceDefinition) (string, error) {
	var storageVersion string
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			storageVersion = v.Name
			break
		}
	}
	if storageVersion == "" {
		return "", errors.Errorf("could not find storage version for CustomResourceDefinition %s", crd.Name)
	}

	return storageVersion, nil
}

func storageVersionMigrationRequired(crd *apiextensionsv1.CustomResourceDefinition, storageVersion string) bool {
	if len(crd.Status.StoredVersions) == 1 && crd.Status.StoredVersions[0] == storageVersion {
		return false
	}
	return true
}

func (r *CRDMigrator) listCustomResources(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition, migrationConfig ByObjectConfig, storageVersion string) ([]client.Object, error) {
	var objs []client.Object

	listGVK := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: storageVersion,
		Kind:    crd.Spec.Names.ListKind,
	}

	if migrationConfig.UseCache {
		// Note: We should only use the cached client with a typed object list.
		// Otherwise we would create an additional informer for an UnstructuredList/PartialObjectMetadataList.
		object, err := r.Client.Scheme().New(listGVK)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list %s: failed to create %s object", crd.Spec.Names.Kind, crd.Spec.Names.ListKind)
		}
		objectList, ok := object.(client.ObjectList)
		if !ok {
			return nil, errors.Wrapf(err, "failed to list %s: %s object is not an ObjectList", crd.Spec.Names.Kind, crd.Spec.Names.ListKind)
		}
		objects, err := listObjectsFromCachedClient(ctx, r.Client, objectList)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list %s via cached client", crd.Spec.Names.Kind)
		}
		objs = append(objs, objects...)
	} else {
		// Note: As the APIReader won't create an informer we can use PartialObjectMetadataList.
		// PartialObjectMetadataList is used instead of UnstructuredList to avoid retrieving unnecessary data.
		objectList := &metav1.PartialObjectMetadataList{}
		objectList.SetGroupVersionKind(listGVK)
		objects, err := listObjectsFromAPIReader(ctx, r.APIReader, objectList)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list %s via live client", crd.Spec.Names.Kind)
		}
		objs = append(objs, objects...)
	}

	return objs, nil
}

func listObjectsFromCachedClient(ctx context.Context, c client.Client, objectList client.ObjectList) ([]client.Object, error) {
	objs := []client.Object{}

	// Note: No pagination needed as we are just reading from the cached client.
	if err := c.List(ctx, objectList); err != nil {
		return nil, err
	}

	objectListItems, err := meta.ExtractList(objectList)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract list items")
	}
	for _, obj := range objectListItems {
		objs = append(objs, obj.(client.Object))
	}

	return objs, nil
}

func listObjectsFromAPIReader(ctx context.Context, c client.Reader, objectList client.ObjectList) ([]client.Object, error) {
	objs := []client.Object{}

	for {
		listOpts := []client.ListOption{
			client.Continue(objectList.GetContinue()),
			client.Limit(500),
		}
		if err := c.List(ctx, objectList, listOpts...); err != nil {
			return nil, err
		}

		objectListItems, err := meta.ExtractList(objectList)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to extract list items")
		}
		for _, obj := range objectListItems {
			objs = append(objs, obj.(client.Object))
		}

		if objectList.GetContinue() == "" {
			break
		}
	}

	return objs, nil
}

func (r *CRDMigrator) reconcileStorageVersionMigration(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition, customResourceObjects []client.Object, storageVersion string) error {
	if len(customResourceObjects) == 0 {
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info(fmt.Sprintf("Running storage version migration to version: %s", storageVersion))

	gvk := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: storageVersion,
		Kind:    crd.Spec.Names.Kind,
	}

	for _, obj := range customResourceObjects {
		e := objectEntry{
			Kind:      gvk.Kind,
			ObjectKey: client.ObjectKeyFromObject(obj),
			// Note: We also have to set the generation of the CRD to ensure we actually run the migration
			// on the latest state of the CRD.
			CRDGeneration: crd.Generation,
		}

		if _, alreadyMigrated := r.storageVersionMigrationCache.Has(e.Key()); alreadyMigrated {
			continue
		}

		// Based on: https://github.com/kubernetes/kubernetes/blob/v1.32.0/pkg/controller/storageversionmigrator/storageversionmigrator.go#L275-L284
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		u.SetNamespace(obj.GetNamespace())
		u.SetName(obj.GetName())
		// Set UID so that when a resource gets deleted, we get an "uid mismatch"
		// conflict error instead of trying to create it.
		u.SetUID(obj.GetUID())
		// Set RV so that when a resources gets updated or deleted+recreated, we get an "object has been modified"
		// conflict error. We do not actually need to do this for the updated case because if RV
		// was not set, it would just result in no-op request. But for the deleted+recreated case, if RV is
		// not set but UID is set, we would get an immutable field validation error. Hence we must set both.
		u.SetResourceVersion(obj.GetResourceVersion())

		log.V(4).Info("Migrating to new storage version", gvk.Kind, klog.KObj(u))
		err := r.Client.Patch(ctx, u, client.Apply, client.FieldOwner("crdmigrator"))
		// If we got a NotFound error, the object no longer exists so no need to update it.
		// If we got a Conflict error, another client wrote the object already so no need to update it.
		if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			return errors.Wrapf(err, "failed to migrate storage version of %s %s", gvk.Kind, klog.KObj(u))
		}

		r.storageVersionMigrationCache.Add(e)
	}

	return nil
}

func (r *CRDMigrator) reconcileCleanupManagedFields(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition, customResourceObjects []client.Object, migrationConfig ByObjectConfig, storageVersion string) error {
	if len(customResourceObjects) == 0 {
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Running managedField cleanup")

	// Collect all GroupVersions that still exist and are served.
	servedGroupVersions := sets.Set[string]{}
	for _, v := range crd.Spec.Versions {
		if v.Served {
			servedGroupVersions.Insert(fmt.Sprintf("%s/%s", crd.Spec.Group, v.Name))
		}
	}

	for _, obj := range customResourceObjects {
		if len(obj.GetManagedFields()) == 0 {
			continue
		}

		var getErr error
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			getErr = nil

			managedFields, removed := removeManagedFieldsWithNotServedGroupVersion(obj, servedGroupVersions)
			if !removed {
				return nil
			}

			if len(managedFields) == 0 {
				// Add a seeding managedFieldEntry for SSA to prevent SSA to create/infer a default managedFieldEntry
				// when the first SSA is applied.
				// More specifically, if an existing object doesn't have managedFields when applying the first SSA
				// the API server creates an entry with operation=Update (kind of guessing where the object comes from),
				// but this entry ends up acting as a co-ownership and we want to prevent this.
				// NOTE: fieldV1Map cannot be empty, so we add metadata.name which will be cleaned up at the first
				//       SSA patch of the same fieldManager.
				// NOTE: We use the fieldManager and operation from the managedFields that we remove to increase the
				//       chances that the managedField entry gets cleaned up. In any case having a minimal entry only
				//       for metadata.name is better than leaving the old entry that uses an apiVersion that is not
				//       served anymore (see: https://github.com/kubernetes/kubernetes/issues/111937).
				fieldV1Map := map[string]interface{}{
					"f:metadata": map[string]interface{}{
						"f:name": map[string]interface{}{},
					},
				}
				fieldV1, err := json.Marshal(fieldV1Map)
				if err != nil {
					return errors.Wrap(err, "failed to create seeding managedField entry")
				}
				managedFields = append(managedFields, metav1.ManagedFieldsEntry{
					Manager:    obj.GetManagedFields()[0].Manager,
					Operation:  obj.GetManagedFields()[0].Operation,
					APIVersion: schema.GroupVersion{Group: crd.Spec.Group, Version: storageVersion}.String(),
					Time:       ptr.To(metav1.Now()),
					FieldsType: "FieldsV1",
					FieldsV1:   &metav1.FieldsV1{Raw: fieldV1},
				})
			}

			// Create a patch to update only managedFields.
			// Include resourceVersion to avoid race conditions.
			jsonPatch := []map[string]interface{}{
				{
					"op":    "replace",
					"path":  "/metadata/managedFields",
					"value": managedFields,
				},
				{
					// Use "replace" instead of "test" operation so that etcd rejects with
					// 409 conflict instead of apiserver with an invalid request
					"op":    "replace",
					"path":  "/metadata/resourceVersion",
					"value": obj.GetResourceVersion(),
				},
			}
			patch, err := json.Marshal(jsonPatch)
			if err != nil {
				return errors.Wrap(err, "failed to marshal patch")
			}

			log.V(4).Info("Cleaning up managedFields", crd.Spec.Names.Kind, klog.KObj(obj))
			err = r.Client.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, patch))
			// If the resource no longer exists, the managedFields don't have to be cleaned up anymore.
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}

			if apierrors.IsConflict(err) {
				if migrationConfig.UseCache {
					getErr = r.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
				} else {
					getErr = r.APIReader.Get(ctx, client.ObjectKeyFromObject(obj), obj)
				}
			}
			// Note: We always have to return the conflict error directly (instead of an aggregate) so retry on conflict works.
			return err
		}); err != nil {
			return errors.Wrapf(kerrors.NewAggregate([]error{err, getErr}), "failed to cleanup managedFields of %s %s", crd.Spec.Names.Kind, klog.KObj(obj))
		}
	}

	return nil
}

func removeManagedFieldsWithNotServedGroupVersion(obj client.Object, servedGroupVersions sets.Set[string]) ([]metav1.ManagedFieldsEntry, bool) {
	removedManagedFields := false
	managedFields := []metav1.ManagedFieldsEntry{}
	for _, managedField := range obj.GetManagedFields() {
		if servedGroupVersions.Has(managedField.APIVersion) {
			managedFields = append(managedFields, managedField)
			continue
		}

		removedManagedFields = true
	}

	return managedFields, removedManagedFields
}

type objectEntry struct {
	Kind string
	client.ObjectKey
	CRDGeneration int64
}

func (r objectEntry) Key() string {
	return fmt.Sprintf("%s %s %d", r.Kind, r.ObjectKey.String(), r.CRDGeneration)
}
