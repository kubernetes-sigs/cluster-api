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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses;clusterclasses/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconciler reconciles the ClusterClass object.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// RuntimeClient is a client for calling runtime extensions.
	RuntimeClient runtimeclient.Client

	// UnstructuredCachingClient provides a client that forces caching of unstructured objects,
	// thus allowing to optimize reads for templates or provider specific objects.
	UnstructuredCachingClient client.Client
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		Named("clusterclass").
		WithOptions(options).
		Watches(
			&runtimev1.ExtensionConfig{},
			handler.EnqueueRequestsFromMapFunc(r.extensionConfigToClusterClass),
		).
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

	patchHelper, err := patch.NewHelper(clusterClass, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for %s", tlog.KObj{Obj: clusterClass})
	}

	defer func() {
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if reterr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, clusterClass, patchOpts...); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, errors.Wrapf(err, "failed to patch %s", tlog.KObj{Obj: clusterClass})})
			return
		}
	}()
	return ctrl.Result{}, r.reconcile(ctx, clusterClass)
}

func (r *Reconciler) reconcile(ctx context.Context, clusterClass *clusterv1.ClusterClass) error {
	if err := r.reconcileVariables(ctx, clusterClass); err != nil {
		return err
	}
	outdatedRefs, err := r.reconcileExternalReferences(ctx, clusterClass)
	if err != nil {
		return err
	}

	reconcileConditions(clusterClass, outdatedRefs)

	return nil
}

func (r *Reconciler) reconcileExternalReferences(ctx context.Context, clusterClass *clusterv1.ClusterClass) (map[*corev1.ObjectReference]*corev1.ObjectReference, error) {
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
	reconciledRefs := sets.Set[string]{}
	outdatedRefs := map[*corev1.ObjectReference]*corev1.ObjectReference{}
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
			outdatedRefs[ref] = updatedRef
		}
	}
	if len(errs) > 0 {
		return nil, kerrors.NewAggregate(errs)
	}
	return outdatedRefs, nil
}

func (r *Reconciler) reconcileVariables(ctx context.Context, clusterClass *clusterv1.ClusterClass) error {
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

			resp := &runtimehooksv1.DiscoverVariablesResponse{}
			err := r.RuntimeClient.CallExtension(ctx, runtimehooksv1.DiscoverVariables, clusterClass, *patch.External.DiscoverVariablesExtension, req, resp)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to call DiscoverVariables for patch %s", patch.Name))
				continue
			}
			if resp.Status != runtimehooksv1.ResponseStatusSuccess {
				errs = append(errs, errors.Errorf("patch %s returned status %q with message %q", patch.Name, resp.Status, resp.Message))
				continue
			}
			if resp.Variables != nil {
				uniqueNamesForPatch := sets.Set[string]{}
				for _, variable := range resp.Variables {
					// Ensure a patch doesn't define multiple variables with the same name.
					if uniqueNamesForPatch.Has(variable.Name) {
						errs = append(errs, errors.Errorf("variable %q is defined multiple times in variable discovery response from patch %q", variable.Name, patch.Name))
						continue
					}
					uniqueNamesForPatch.Insert(variable.Name)

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
		// TODO: Decide whether to remove old variables if discovery fails.
		conditions.MarkFalse(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition, clusterv1.VariableDiscoveryFailedReason, clusterv1.ConditionSeverityError,
			"VariableDiscovery failed: %s", kerrors.NewAggregate(errs))
		return errors.Wrapf(kerrors.NewAggregate(errs), "failed to discover variables for ClusterClass %s", clusterClass.Name)
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
	conditions.MarkTrue(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
	return nil
}

func reconcileConditions(clusterClass *clusterv1.ClusterClass, outdatedRefs map[*corev1.ObjectReference]*corev1.ObjectReference) {
	if len(outdatedRefs) > 0 {
		var msg []string
		for currentRef, updatedRef := range outdatedRefs {
			msg = append(msg, fmt.Sprintf("Ref %q should be %q", refString(currentRef), refString(updatedRef)))
		}
		conditions.Set(
			clusterClass,
			conditions.FalseCondition(
				clusterv1.ClusterClassRefVersionsUpToDateCondition,
				clusterv1.ClusterClassOutdatedRefVersionsReason,
				clusterv1.ConditionSeverityWarning,
				strings.Join(msg, ", "),
			),
		)
		return
	}

	conditions.Set(
		clusterClass,
		conditions.TrueCondition(clusterv1.ClusterClassRefVersionsUpToDateCondition),
	)
}

func addNewStatusVariable(variable clusterv1.ClusterClassVariable, from string) *clusterv1.ClusterClassStatusVariable {
	return &clusterv1.ClusterClassStatusVariable{
		Name:                variable.Name,
		DefinitionsConflict: false,
		Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
			{
				From:     from,
				Required: variable.Required,
				Schema:   variable.Schema,
			},
		}}
}

func addDefinitionToExistingStatusVariable(variable clusterv1.ClusterClassVariable, from string, existingVariable *clusterv1.ClusterClassStatusVariable) *clusterv1.ClusterClassStatusVariable {
	combinedVariable := existingVariable.DeepCopy()
	newVariableDefinition := clusterv1.ClusterClassStatusVariableDefinition{
		From:     from,
		Required: variable.Required,
		Schema:   variable.Schema,
	}
	combinedVariable.Definitions = append(existingVariable.Definitions, newVariableDefinition)

	// If the new definition is different from any existing definition, set DefinitionsConflict to true.
	// If definitions already conflict, no need to check.
	if !combinedVariable.DefinitionsConflict {
		currentDefinition := combinedVariable.Definitions[0]
		if !(currentDefinition.Required == newVariableDefinition.Required && reflect.DeepEqual(currentDefinition.Schema, newVariableDefinition.Schema)) {
			combinedVariable.DefinitionsConflict = true
		}
	}
	return combinedVariable
}

func refString(ref *corev1.ObjectReference) string {
	return fmt.Sprintf("%s %s/%s", ref.GroupVersionKind().String(), ref.Namespace, ref.Name)
}

func (r *Reconciler) reconcileExternal(ctx context.Context, clusterClass *clusterv1.ClusterClass, ref *corev1.ObjectReference) error {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, clusterClass.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return errors.Wrapf(err, "Could not find external object for the ClusterClass. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
		}
		return errors.Wrapf(err, "failed to get the external object for the ClusterClass. refGroupVersionKind: %s, refName: %s", ref.GroupVersionKind(), ref.Name)
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
		return errors.Wrapf(err, "failed to set ClusterClass owner reference for %s", tlog.KObj{Obj: obj})
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
				extName, err := runtimeclient.ExtensionNameFromHandlerName(*patch.External.DiscoverVariablesExtension)
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
