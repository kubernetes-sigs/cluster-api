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
	"reflect"
	"sort"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	internalruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses;clusterclasses/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles the ClusterClass object.
type Reconciler struct {
	Client client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// RuntimeClient is a client for calling runtime extensions.
	RuntimeClient runtimeclient.Client

	// discoverVariablesCache is used to temporarily store the response of a DiscoveryVariables call for
	// a specific runtime extension/settings combination.
	discoverVariablesCache cache.Cache[runtimeclient.CallExtensionCacheEntry]
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil {
		return errors.New("Client must not be nil")
	}
	if feature.Gates.Enabled(feature.RuntimeSDK) && r.RuntimeClient == nil {
		return errors.New("RuntimeClient must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "clusterclass")
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		WithOptions(options).
		Watches(
			&runtimev1.ExtensionConfig{},
			handler.EnqueueRequestsFromMapFunc(r.extensionConfigToClusterClass),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Complete(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.discoverVariablesCache = cache.New[runtimeclient.CallExtensionCacheEntry]()
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (retres ctrl.Result, reterr error) {
	clusterClass := &clusterv1.ClusterClass{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterClass); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Error reading the object  - requeue the request.
		return ctrl.Result{}, err
	}

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, nil, clusterClass); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	if !clusterClass.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	s := &scope{
		clusterClass: clusterClass,
	}

	patchHelper, err := patch.NewHelper(clusterClass, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		updateStatus(ctx, s)

		patchOpts := []patch.Option{
			patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.ClusterClassRefVersionsUpToDateCondition,
				clusterv1.ClusterClassVariablesReconciledCondition,
			}},
			patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.ClusterClassRefVersionsUpToDateV1Beta2Condition,
				clusterv1.ClusterClassVariablesReadyV1Beta2Condition,
			}},
		}

		// Patch ObservedGeneration only if the reconciliation completed successfully
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, clusterClass, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
			return
		}

		if reterr != nil {
			retres = ctrl.Result{}
		}
	}()

	reconcileNormal := []clusterClassReconcileFunc{
		r.reconcileExternalReferences,
		r.reconcileVariables,
	}
	return doReconcile(ctx, reconcileNormal, s)
}

type clusterClassReconcileFunc func(context.Context, *scope) (ctrl.Result, error)

func doReconcile(ctx context.Context, phases []clusterClassReconcileFunc, s *scope) (ctrl.Result, error) {
	res := ctrl.Result{}
	errs := []error{}
	for _, phase := range phases {
		// Call the inner reconciliation methods.
		phaseResult, err := phase(ctx, s)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		res = util.LowestNonZeroResult(res, phaseResult)
	}

	if len(errs) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errs)
	}

	return res, nil
}

// scope holds the different objects that are read and used during the reconcile.
type scope struct {
	// clusterClass is the ClusterClass object being reconciled.
	// It is set at the beginning of the reconcile function.
	clusterClass *clusterv1.ClusterClass

	reconcileExternalReferencesError error
	outdatedExternalReferences       []outdatedRef

	variableDiscoveryError error
}

type outdatedRef struct {
	Outdated *corev1.ObjectReference
	UpToDate *corev1.ObjectReference
}

func (r *Reconciler) reconcileExternalReferences(ctx context.Context, s *scope) (ctrl.Result, error) {
	clusterClass := s.clusterClass

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

	for _, mpClass := range clusterClass.Spec.Workers.MachinePools {
		if mpClass.Template.Bootstrap.Ref != nil {
			refs = append(refs, mpClass.Template.Bootstrap.Ref)
		}
		if mpClass.Template.Infrastructure.Ref != nil {
			refs = append(refs, mpClass.Template.Infrastructure.Ref)
		}
	}

	// Ensure all referenced objects are owned by the ClusterClass.
	// Nb. Some external objects can be referenced multiple times in the ClusterClass,
	// but we only want to set the owner reference once per unique external object.
	// For example the same KubeadmConfigTemplate could be referenced in multiple MachineDeployment
	// or MachinePool classes.
	errs := []error{}
	reconciledRefs := sets.Set[string]{}
	outdatedRefs := []outdatedRef{}
	for i := range refs {
		ref := refs[i]
		uniqueKey := uniqueObjectRefKey(ref)

		// Continue as we only have to reconcile every referenced object once.
		if reconciledRefs.Has(uniqueKey) {
			continue
		}

		reconciledRefs.Insert(uniqueKey)

		// Add the ClusterClass as owner reference to the templates so clusterctl move
		// can identify all related objects and Kubernetes garbage collector deletes
		// all referenced templates on ClusterClass deletion.
		if err := r.reconcileExternal(ctx, clusterClass, ref); err != nil {
			errs = append(errs, err)
			continue
		}

		// Check if the template reference is outdated, i.e. it is not using the latest apiVersion
		// for the current CAPI contract.
		updatedRef := ref.DeepCopy()
		if err := conversion.UpdateReferenceAPIContract(ctx, r.Client, updatedRef); err != nil {
			errs = append(errs, err)
		}
		if ref.GroupVersionKind().Version != updatedRef.GroupVersionKind().Version {
			outdatedRefs = append(outdatedRefs, outdatedRef{
				Outdated: ref,
				UpToDate: updatedRef,
			})
		}
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		s.reconcileExternalReferencesError = err
		return ctrl.Result{}, err
	}

	s.outdatedExternalReferences = outdatedRefs
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileVariables(ctx context.Context, s *scope) (ctrl.Result, error) {
	clusterClass := s.clusterClass

	errs := []error{}
	allVariableDefinitions := map[string]*clusterv1.ClusterClassStatusVariable{}
	// Add inline variable definitions to the ClusterClass status.
	for _, variable := range clusterClass.Spec.Variables {
		allVariableDefinitions[variable.Name] = addNewStatusVariable(variable, clusterv1.VariableDefinitionFromInline)
	}

	// If RuntimeSDK is enabled call the DiscoverVariables hook for all associated Runtime Extensions and add the variables
	// to the ClusterClass status.
	if feature.Gates.Enabled(feature.RuntimeSDK) {
		for _, patch := range clusterClass.Spec.Patches {
			if patch.External == nil || patch.External.DiscoverVariablesExtension == nil {
				continue
			}
			req := &runtimehooksv1.DiscoverVariablesRequest{}
			req.Settings = patch.External.Settings

			// We temporarily cache the response of a DiscoveryVariables call to improve performance in case there are
			// many ClusterClasses using the same runtime extension/settings combination.
			// This also mitigates spikes when ClusterClass re-syncs happen or when changes to the ExtensionConfig are applied.
			// DiscoverVariables is expected to return a "static" response and usually there are few ExtensionConfigs in a mgmt cluster.
			resp := &runtimehooksv1.DiscoverVariablesResponse{}
			err := r.RuntimeClient.CallExtension(ctx, runtimehooksv1.DiscoverVariables, clusterClass, *patch.External.DiscoverVariablesExtension, req, resp,
				runtimeclient.WithCaching{Cache: r.discoverVariablesCache, CacheKeyFunc: cacheKeyFunc})
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to call DiscoverVariables for patch %s", patch.Name))
				continue
			}
			if resp.Status != runtimehooksv1.ResponseStatusSuccess {
				errs = append(errs, errors.Errorf("patch %s returned status %q with message %q", patch.Name, resp.Status, resp.Message))
				continue
			}
			if resp.Variables != nil {
				validationErrors := variables.ValidateClusterClassVariables(ctx, nil, resp.Variables, field.NewPath(patch.Name, "variables")).ToAggregate()
				if validationErrors != nil {
					errs = append(errs, validationErrors)
					continue
				}

				for _, variable := range resp.Variables {
					// If a variable of the same name already exists in allVariableDefinitions add the new definition to the existing list.
					if _, ok := allVariableDefinitions[variable.Name]; ok {
						allVariableDefinitions[variable.Name] = addDefinitionToExistingStatusVariable(variable, patch.Name, allVariableDefinitions[variable.Name])
						continue
					}

					// Add the new variable to the list.
					allVariableDefinitions[variable.Name] = addNewStatusVariable(variable, patch.Name)
				}
			}
		}
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		s.variableDiscoveryError = errors.Wrapf(err, "VariableDiscovery failed")
		// TODO: Decide whether to remove old variables if discovery fails.
		return ctrl.Result{}, errors.Wrapf(err, "failed to discover variables for ClusterClass %s", clusterClass.Name)
	}

	statusVarList := []clusterv1.ClusterClassStatusVariable{}
	for _, variable := range allVariableDefinitions {
		statusVarList = append(statusVarList, *variable)
	}
	// Alphabetically sort the variables by name. This ensures no unnecessary updates to the ClusterClass status.
	// Note: Definitions in `statusVarList[i].Definitions` have a stable order as they are added in a deterministic order
	// and are always held in an array.
	sort.SliceStable(statusVarList, func(i, j int) bool {
		return statusVarList[i].Name < statusVarList[j].Name
	})
	clusterClass.Status.Variables = statusVarList

	variablesWithConflict := []string{}
	for _, v := range clusterClass.Status.Variables {
		if v.DefinitionsConflict {
			variablesWithConflict = append(variablesWithConflict, v.Name)
		}
	}

	if len(variablesWithConflict) > 0 {
		err := fmt.Errorf("the following variables have conflicting schemas: %s", strings.Join(variablesWithConflict, ","))
		s.variableDiscoveryError = errors.Wrapf(err, "VariableDiscovery failed")
		return ctrl.Result{}, errors.Wrapf(err, "failed to discover variables for ClusterClass %s", clusterClass.Name)
	}

	return ctrl.Result{}, nil
}

func addNewStatusVariable(variable clusterv1.ClusterClassVariable, from string) *clusterv1.ClusterClassStatusVariable {
	return &clusterv1.ClusterClassStatusVariable{
		Name:                variable.Name,
		DefinitionsConflict: false,
		Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
			{
				From:     from,
				Required: variable.Required,
				Metadata: variable.Metadata,
				Schema:   variable.Schema,
			},
		}}
}

func addDefinitionToExistingStatusVariable(variable clusterv1.ClusterClassVariable, from string, existingVariable *clusterv1.ClusterClassStatusVariable) *clusterv1.ClusterClassStatusVariable {
	combinedVariable := existingVariable.DeepCopy()
	newVariableDefinition := clusterv1.ClusterClassStatusVariableDefinition{
		From:     from,
		Required: variable.Required,
		Metadata: variable.Metadata,
		Schema:   variable.Schema,
	}
	combinedVariable.Definitions = append(existingVariable.Definitions, newVariableDefinition)

	// If the new definition is different from any existing definition, set DefinitionsConflict to true.
	// If definitions already conflict, no need to check.
	if !combinedVariable.DefinitionsConflict {
		currentDefinition := combinedVariable.Definitions[0]
		if !(currentDefinition.Required == newVariableDefinition.Required && reflect.DeepEqual(currentDefinition.Schema, newVariableDefinition.Schema) && reflect.DeepEqual(currentDefinition.Metadata, newVariableDefinition.Metadata)) {
			combinedVariable.DefinitionsConflict = true
		}
	}
	return combinedVariable
}

func refString(ref *corev1.ObjectReference) string {
	return fmt.Sprintf("%s %s/%s", ref.GroupVersionKind().String(), ref.Namespace, ref.Name)
}

func (r *Reconciler) reconcileExternal(ctx context.Context, clusterClass *clusterv1.ClusterClass, ref *corev1.ObjectReference) error {
	obj, err := external.Get(ctx, r.Client, ref)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return errors.Wrapf(err, "Could not find external object for the ClusterClass. refGroupVersionKind: %s, refName: %s, refNamespace: %s", ref.GroupVersionKind(), ref.Name, ref.Namespace)
		}
		return errors.Wrapf(err, "failed to get the external object for the ClusterClass. refGroupVersionKind: %s, refName: %s, refNamespace: %s", ref.GroupVersionKind(), ref.Name, ref.Namespace)
	}

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "ClusterClass",
		Name:       clusterClass.Name,
		UID:        clusterClass.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}

	if err := controllerutil.SetOwnerReference(clusterClass, obj, r.Client.Scheme()); err != nil {
		return errors.Wrapf(err, "failed to set ClusterClass owner reference for %s %s", obj.GetKind(), klog.KObj(obj))
	}

	return patchHelper.Patch(ctx, obj)
}

func uniqueObjectRefKey(ref *corev1.ObjectReference) string {
	return fmt.Sprintf("Name:%s, Namespace:%s, Kind:%s, APIVersion:%s", ref.Name, ref.Namespace, ref.Kind, ref.APIVersion)
}

// extensionConfigToClusterClass maps an ExtensionConfigs to the corresponding ClusterClass to reconcile them on updates
// of the ExtensionConfig.
func (r *Reconciler) extensionConfigToClusterClass(ctx context.Context, o client.Object) []reconcile.Request {
	res := []ctrl.Request{}
	log := ctrl.LoggerFrom(ctx)
	ext, ok := o.(*runtimev1.ExtensionConfig)
	if !ok {
		panic(fmt.Sprintf("Expected an ExtensionConfig but got a %T", o))
	}

	clusterClasses := clusterv1.ClusterClassList{}
	selector, err := metav1.LabelSelectorAsSelector(ext.Spec.NamespaceSelector)
	if err != nil {
		return nil
	}
	if err := r.Client.List(ctx, &clusterClasses); err != nil {
		return nil
	}
	for _, clusterClass := range clusterClasses.Items {
		if !matchNamespace(ctx, r.Client, selector, clusterClass.Namespace) {
			continue
		}
		for _, patch := range clusterClass.Spec.Patches {
			if patch.External != nil && patch.External.DiscoverVariablesExtension != nil {
				extName, err := internalruntimeclient.ExtensionNameFromHandlerName(*patch.External.DiscoverVariablesExtension)
				if err != nil {
					log.Error(err, "failed to reconcile ClusterClass for ExtensionConfig")
					continue
				}
				if extName == ext.Name {
					res = append(res, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: clusterClass.Namespace, Name: clusterClass.Name}})
					// Once we've added the ClusterClass once we can break here.
					break
				}
			}
		}
	}
	return res
}

// matchNamespace returns true if the passed namespace matches the selector.
func matchNamespace(ctx context.Context, c client.Client, selector labels.Selector, namespace string) bool {
	// Return early if the selector is empty.
	if selector.Empty() {
		return true
	}

	ns := &metav1.PartialObjectMetadata{}
	ns.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	})
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
		return false
	}
	return selector.Matches(labels.Set(ns.GetLabels()))
}

func cacheKeyFunc(extensionName, extensionConfigResourceVersion string, request runtimehooksv1.RequestObject) string {
	// Note: registration.Name is identical to the value of the patch.External.DiscoverVariablesExtension field in the ClusterClass.
	s := fmt.Sprintf("%s-%s", extensionName, extensionConfigResourceVersion)
	for k, v := range request.GetSettings() {
		s += fmt.Sprintf(",%s=%s", k, v)
	}
	return s
}
