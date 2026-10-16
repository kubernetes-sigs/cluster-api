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

package machinedeployment

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/internal/util/patch"
	"sigs.k8s.io/cluster-api/util/cache"
)

// CanUpdateMachineSetCacheEntry is an entry for the CanUpdateMachineSet hook cache.
type CanUpdateMachineSetCacheEntry struct {
	OldMS client.ObjectKey
	NewMS client.ObjectKey

	// canUpdateMachineSetResult is the cached CanUpdateMachineSet result.
	canUpdateMachineSetResult
}

// Key returns the cache key of a CanUpdateMachineSetCacheEntry.
func (r CanUpdateMachineSetCacheEntry) Key() string {
	return fmt.Sprintf("%s => %s", r.OldMS, r.NewMS)
}

// canUpdateMachineSetCacheDuration is the duration for which CanUpdateMachineSet results are cached
// (if the extension did not return retryAfterSeconds).
const canUpdateMachineSetCacheDuration = 5 * time.Minute

type canUpdateMachineSetResult struct {
	// retryAfter is the time at which the result expires and we should reconcile again (and call canUpdateMachineSetResult extensions again).
	// Note: This is either the retry requested by an extension via retryAfterSeconds (see retryAfterFromExtension)
	// or the time at which the cached result expires.
	retryAfter time.Time

	// retryAfterFromExtension is true if an extension returned retryAfterSeconds, i.e. the decision is still in progress.
	// Note: This is needed to differentiate between retryAfterSeconds>0 and other cases.
	retryAfterFromExtension bool

	canUpdateMachineSet bool
	affectsAvailability bool

	// allowedOperations are the operations that are allowed by all extensions.
	// Note: The zero value (nil) means that the extensions have no opinion, i.e. all operations are allowed.
	// An empty list means that no operation is allowed.
	allowedOperations *[]runtimehooksv1.MachineUpdateOperation

	// messages are the messages from the CanUpdateMachineSet responses.
	messages []string

	// reasons are the reasons why the MachineSet cannot be updated in-place.
	reasons []string

	// internalErrorMessage is the error message that is set, together with the returned error, if calling the extensions failed.
	// Note: This is never cached, errors are not cached so the extensions are called again on the next reconcile.
	internalErrorMessage string
}

func (p *rolloutPlanner) canUpdateMachineSetInPlace(ctx context.Context, oldMS, newMS *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
	if p.overrideCanUpdateMachineSetInPlace != nil {
		return p.overrideCanUpdateMachineSetInPlace(ctx, oldMS, newMS)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("MachineSet", klog.KObj(oldMS))

	templateObjects, err := p.getTemplateObjects(ctx, oldMS, newMS)
	if err != nil {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, err
	}

	// MachineSet cannot be updated in-place if the getTemplateObjects func was not able to get all InfraMachineTemplates.
	if templateObjects.CurrentInfraMachineTemplate == nil ||
		templateObjects.DesiredInfraMachineTemplate == nil {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, err
	}
	// MachineSet cannot be updated in-place if the BootstrapConfigTemplate is set on the oldMS but not on the newMS or vice versa.
	if (templateObjects.CurrentBootstrapConfigTemplate == nil && templateObjects.DesiredBootstrapConfigTemplate != nil) ||
		(templateObjects.CurrentBootstrapConfigTemplate != nil && templateObjects.DesiredBootstrapConfigTemplate == nil) {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, err
	}

	extensionHandlers, err := p.RuntimeClient.GetAllExtensions(ctx, runtimehooksv1.CanUpdateMachineSet, oldMS)
	if err != nil {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, err
	}
	// MachineSet cannot be updated in-place if no CanUpdateMachineSet extensions are registered.
	if len(extensionHandlers) == 0 {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
	}
	if len(extensionHandlers) > 1 {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("found multiple CanUpdateMachineSet hooks (%s): only one hook is supported", strings.Join(extensionHandlers, ","))
	}

	var res canUpdateMachineSetResult
	logSuffix := ""
	if entry := p.tryToGetResultsFromCache(p.canUpdateMachineSetCache, oldMS, newMS); entry != nil {
		// Get result from cache.
		res = entry.canUpdateMachineSetResult

		// Refresh the cache entry, so only tryToGetResultsFromCache decides when a cache entry should not be used anymore.
		p.canUpdateMachineSetCache.Add(*entry)

		// Note: Cached results are only logged on log level 5.
		log = log.V(5)
		logSuffix = " (cached)"
	} else {
		// Get result from extensions.
		var err error
		res, err = p.canExtensionsUpdateMachineSet(ctx, oldMS, newMS, templateObjects, extensionHandlers)
		if err != nil {
			return canUpdateMachineSetResult{
				canUpdateMachineSet:  false,
				internalErrorMessage: err.Error(),
			}, err
		}

		p.canUpdateMachineSetCache.Add(CanUpdateMachineSetCacheEntry{
			OldMS:                     client.ObjectKeyFromObject(oldMS),
			NewMS:                     client.ObjectKeyFromObject(newMS),
			canUpdateMachineSetResult: res,
		})
	}

	if len(res.reasons) > 0 {
		log = log.WithValues("reason", strings.Join(res.reasons, ","))
	}
	if len(res.messages) > 0 {
		log = log.WithValues("messages", strings.Join(res.messages, ","))
	}
	switch {
	case res.retryAfterFromExtension:
		log.Info(fmt.Sprintf("MachineSet %s cannot be rolled out yet: extensions returned retryAfterSeconds, requeuing%s", klog.KObj(oldMS), logSuffix))
	case !res.canUpdateMachineSet:
		log.Info(fmt.Sprintf("MachineSet %s cannot be updated in-place by extensions%s", klog.KObj(oldMS), logSuffix))
	case res.affectsAvailability:
		log.Info(fmt.Sprintf("MachineSet %s can be updated in-place by extensions (affects availability)%s", klog.KObj(oldMS), logSuffix))
	default:
		log.Info(fmt.Sprintf("MachineSet %s can be updated in-place by extensions (does not affect availability)%s", klog.KObj(oldMS), logSuffix))
	}
	return res, nil
}

func (p *rolloutPlanner) tryToGetResultsFromCache(cache cache.Cache[CanUpdateMachineSetCacheEntry], oldMS, newMS *clusterv1.MachineSet) *CanUpdateMachineSetCacheEntry {
	entry, ok := cache.Has(CanUpdateMachineSetCacheEntry{
		OldMS: client.ObjectKeyFromObject(oldMS),
		NewMS: client.ObjectKeyFromObject(newMS),
	}.Key())

	// If cache entry does not exist, don't use the cache.
	// NOTE: this covers also the use case that the MachineDeployment is changed, because this will imply NewMS to change and thus a cache miss.
	if !ok {
		return nil
	}

	// If retryAfter is set and it is in the past, don't use the cache.
	if !entry.retryAfter.IsZero() && entry.retryAfter.Before(p.now()) {
		return nil
	}

	// If retryAfter is in the future, use the cache.
	return &entry
}

// canExtensionsUpdateMachineSet calls CanUpdateMachineSet extensions to decide if a MachineSet can be updated in-place.
// Note: This is following the same general structure that is used in the Apply func in
// internal/controllers/topology/cluster/patches/engine.go.
func (p *rolloutPlanner) canExtensionsUpdateMachineSet(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
	if p.overrideCanExtensionsUpdateMachineSet != nil {
		return p.overrideCanExtensionsUpdateMachineSet(ctx, oldMS, newMS, templateObjects, extensionHandlers)
	}

	// Create the CanUpdateMachineSet request.
	req, err := createRequest(oldMS, newMS, templateObjects)
	if err != nil {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Wrapf(err, "failed to generate CanUpdateMachineSet request")
	}

	// Note: We always call all extensions (instead of stopping as soon as the current and desired objects are matching),
	// because every extension can have an opinion about the allowed operations and every extension
	// is able to block (independent of what it is able to update in-place).
	var messages []string
	var affectsAvailability bool
	var allowedOperations *[]runtimehooksv1.MachineUpdateOperation
	for _, extensionHandler := range extensionHandlers {
		// Call CanUpdateMachineSet extension.
		resp := &runtimehooksv1.CanUpdateMachineSetResponse{}
		if err := p.RuntimeClient.CallExtension(ctx, runtimehooksv1.CanUpdateMachineSet, oldMS, extensionHandler, req, resp); err != nil {
			return canUpdateMachineSetResult{canUpdateMachineSet: false}, err
		}

		if resp.Message != "" {
			messages = append(messages, resp.Message)
		}

		// If the extension asks to be called again later, stop here and requeue.
		// Note: In this case the allowedOperations of the extension are not considered, the decision is still in progress.
		if resp.RetryAfterSeconds > 0 {
			return canUpdateMachineSetResult{
				retryAfter:              p.now().Add(time.Duration(resp.RetryAfterSeconds) * time.Second),
				retryAfterFromExtension: true,
				messages:                messages,
			}, nil
		}

		// An operation is only allowed if it is allowed by all extensions.
		allowedOperations = intersectAllowedOperations(allowedOperations, resp.AllowedOperations)

		// If no operation is allowed, the rollout is blocked: stop here, no other extension can change that.
		if allowedOperations != nil && len(*allowedOperations) == 0 {
			return canUpdateMachineSetResult{
				retryAfter:          p.now().Add(canUpdateMachineSetCacheDuration),
				canUpdateMachineSet: false,
				allowedOperations:   allowedOperations,
				messages:            messages,
				reasons:             []string{"no operation is allowed by extensions"},
			}, nil
		}

		affectsAvailability = affectsAvailability || ptr.Deref(resp.AffectsAvailability, true)

		// Apply patches from the CanUpdateMachineSet response to the request.
		if err := applyPatchesToRequest(ctx, req, resp); err != nil {
			return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Wrapf(err, "failed to apply patches from extension %s to the CanUpdateMachineSet request", extensionHandler)
		}
	}

	// Check if current and desired objects are now matching.
	matches, reasons, err := matchesMachineSet(req)
	if err != nil {
		return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Wrapf(err, "failed to compare current and desired objects after calling extensions %s", strings.Join(extensionHandlers, ","))
	}

	return canUpdateMachineSetResult{
		retryAfter:          p.now().Add(canUpdateMachineSetCacheDuration),
		canUpdateMachineSet: matches,
		affectsAvailability: matches && affectsAvailability,
		allowedOperations:   allowedOperations,
		messages:            messages,
		reasons:             reasons,
	}, nil
}

// intersectAllowedOperations returns the operations that are allowed by both a and b.
// Note: nil means no opinion, i.e. all operations are allowed. An empty list means that no operation is allowed.
// The result is nil if both a and b are nil. Otherwise the result is a pointer to a list that only contains
// known operations without duplicates. Unknown operations are ignored.
func intersectAllowedOperations(a, b *[]runtimehooksv1.MachineUpdateOperation) *[]runtimehooksv1.MachineUpdateOperation {
	if a == nil && b == nil {
		return nil
	}

	result := []runtimehooksv1.MachineUpdateOperation{}
	for _, operation := range runtimehooksv1.AllMachineUpdateOperations {
		if isOperationAllowed(a, operation) && isOperationAllowed(b, operation) {
			result = append(result, operation)
		}
	}
	return &result
}

// isOperationAllowed returns true if the operation is allowed according to allowedOperations.
// Note: If allowedOperations is not set the extensions have no opinion, i.e. all operations are allowed.
func isOperationAllowed(allowedOperations *[]runtimehooksv1.MachineUpdateOperation, operation runtimehooksv1.MachineUpdateOperation) bool {
	return allowedOperations == nil || slices.Contains(*allowedOperations, operation)
}

func createRequest(oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects) (*runtimehooksv1.CanUpdateMachineSetRequest, error) {
	// DeepCopy MachineSets to avoid mutations.
	currentMachineSetForDiff := oldMS.DeepCopy()
	currentBootstrapConfigTemplateForDiff := templateObjects.CurrentBootstrapConfigTemplate
	currentInfraMachineTemplateForDiff := templateObjects.CurrentInfraMachineTemplate

	desiredMachineSetForDiff := newMS.DeepCopy()
	desiredBootstrapConfigTemplateForDiff := templateObjects.DesiredBootstrapConfigTemplate
	desiredInfraMachineTemplateForDiff := templateObjects.DesiredInfraMachineTemplate

	// Cleanup objects and create request.
	req := &runtimehooksv1.CanUpdateMachineSetRequest{
		Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet: *cleanupMachineSet(currentMachineSetForDiff),
		},
		Desired: runtimehooksv1.CanUpdateMachineSetRequestObjects{
			MachineSet: *cleanupMachineSet(desiredMachineSetForDiff),
		},
	}
	var err error
	if currentBootstrapConfigTemplateForDiff != nil {
		req.Current.BootstrapConfigTemplate, err = patch.ConvertToRawExtension(cleanupUnstructured(currentBootstrapConfigTemplateForDiff))
		if err != nil {
			return nil, err
		}
	}
	if desiredBootstrapConfigTemplateForDiff != nil {
		req.Desired.BootstrapConfigTemplate, err = patch.ConvertToRawExtension(cleanupUnstructured(desiredBootstrapConfigTemplateForDiff))
		if err != nil {
			return nil, err
		}
	}
	req.Current.InfrastructureMachineTemplate, err = patch.ConvertToRawExtension(cleanupUnstructured(currentInfraMachineTemplateForDiff))
	if err != nil {
		return nil, err
	}
	req.Desired.InfrastructureMachineTemplate, err = patch.ConvertToRawExtension(cleanupUnstructured(desiredInfraMachineTemplateForDiff))
	if err != nil {
		return nil, err
	}

	return req, nil
}

func cleanupMachineSet(machineSet *clusterv1.MachineSet) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		// Set GVK because object is later marshalled with json.Marshal.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        machineSet.Name,
			Namespace:   machineSet.Namespace,
			Labels:      machineSet.Labels,
			Annotations: machineSet.Annotations,
		},
		Spec: *machineSet.Spec.DeepCopy(),
	}
}

func cleanupUnstructured(u *unstructured.Unstructured) *unstructured.Unstructured {
	cleanedUpU := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"spec":       u.Object["spec"],
		},
	}
	cleanedUpU.SetName(u.GetName())
	cleanedUpU.SetNamespace(u.GetNamespace())
	cleanedUpU.SetLabels(u.GetLabels())
	cleanedUpU.SetAnnotations(u.GetAnnotations())
	return cleanedUpU
}

func applyPatchesToRequest(ctx context.Context, req *runtimehooksv1.CanUpdateMachineSetRequest, resp *runtimehooksv1.CanUpdateMachineSetResponse) error {
	if resp.MachineSetPatch.IsDefined() {
		if err := patch.ApplyPatchToTypedObject(ctx, &req.Current.MachineSet, resp.MachineSetPatch, []string{"spec", "template", "spec"}); err != nil {
			return err
		}
	}

	if resp.BootstrapConfigTemplatePatch.IsDefined() && req.Current.BootstrapConfigTemplate.Object != nil {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.BootstrapConfigTemplate, resp.BootstrapConfigTemplatePatch, []string{"spec", "template", "spec"}); err != nil {
			return err
		}
	}

	if resp.InfrastructureMachineTemplatePatch.IsDefined() {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.InfrastructureMachineTemplate, resp.InfrastructureMachineTemplatePatch, []string{"spec", "template", "spec"}); err != nil {
			return err
		}
	}

	return nil
}

func matchesMachineSet(req *runtimehooksv1.CanUpdateMachineSetRequest) (bool, []string, error) {
	var reasons []string
	match, diff, err := matchesMachineSetSpec(&req.Current.MachineSet, &req.Desired.MachineSet)
	if err != nil {
		return false, nil, pkgerrors.Wrapf(err, "failed to match MachineSet")
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("MachineSet cannot be updated in-place: %s", diff))
	}
	if req.Current.BootstrapConfigTemplate.Object != nil && req.Desired.BootstrapConfigTemplate.Object != nil {
		match, diff, err = matchesUnstructuredSpec(req.Current.BootstrapConfigTemplate, req.Desired.BootstrapConfigTemplate)
		if err != nil {
			return false, nil, pkgerrors.Wrapf(err, "failed to match %s", req.Current.BootstrapConfigTemplate.Object.GetObjectKind().GroupVersionKind().Kind)
		}
		if !match {
			reasons = append(reasons, fmt.Sprintf("%s cannot be updated in-place: %s", req.Current.BootstrapConfigTemplate.Object.GetObjectKind().GroupVersionKind().Kind, diff))
		}
	}
	match, diff, err = matchesUnstructuredSpec(req.Current.InfrastructureMachineTemplate, req.Desired.InfrastructureMachineTemplate)
	if err != nil {
		return false, nil, pkgerrors.Wrapf(err, "failed to match %s", req.Current.InfrastructureMachineTemplate.Object.GetObjectKind().GroupVersionKind().Kind)
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("%s cannot be updated in-place: %s", req.Current.InfrastructureMachineTemplate.Object.GetObjectKind().GroupVersionKind().Kind, diff))
	}

	if len(reasons) > 0 {
		return false, reasons, nil
	}

	return true, nil, nil
}

func matchesMachineSetSpec(patched, desired *clusterv1.MachineSet) (equal bool, diff string, matchErr error) {
	// Note: Wrapping MachineSet specs in a MachineSet for proper formatting of the diff.
	return compare.Diff(
		&clusterv1.MachineSet{
			Spec: clusterv1.MachineSetSpec{
				Template: clusterv1.MachineTemplateSpec{
					Spec: *inplace.CleanupMachineSpecForDiff(&patched.Spec.Template.Spec),
				},
			},
		}, &clusterv1.MachineSet{
			Spec: clusterv1.MachineSetSpec{
				Template: clusterv1.MachineTemplateSpec{
					Spec: *inplace.CleanupMachineSpecForDiff(&desired.Spec.Template.Spec),
				},
			},
		},
	)
}

func matchesUnstructuredSpec(patched, desired runtime.RawExtension) (equal bool, diff string, matchErr error) {
	// Note: Both patched and desired objects are always Unstructured as createRequest and
	//       applyPatchToObject are always setting objects as Unstructured.
	patchedUnstructured, ok := patched.Object.(*unstructured.Unstructured)
	if !ok {
		return false, "", pkgerrors.Errorf("patched object is not an Unstructured")
	}
	desiredUnstructured, ok := desired.Object.(*unstructured.Unstructured)
	if !ok {
		return false, "", pkgerrors.Errorf("desired object is not an Unstructured")
	}

	// Note: Wrapping Unstructured spec.template.specs in an Unstructured for proper formatting of the diff.
	patchedSpecTemplateSpec, foundPatched, err := unstructured.NestedFieldNoCopy(patchedUnstructured.Object, "spec", "template", "spec")
	if err != nil {
		return false, "", pkgerrors.Errorf("could not read spec.template.spec from patched object")
	}
	desiredSpecTemplateSpec, foundDesired, err := unstructured.NestedFieldNoCopy(desiredUnstructured.Object, "spec", "template", "spec")
	if err != nil {
		return false, "", pkgerrors.Errorf("could not read spec.template.spec from desired object")
	}

	cleanedUpPatchedUnstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if foundPatched {
		if err := unstructured.SetNestedField(cleanedUpPatchedUnstructured.Object, patchedSpecTemplateSpec, "spec", "template", "spec"); err != nil {
			return false, "", pkgerrors.Errorf("could not write spec.template.spec to patched object for comparison")
		}
	}
	cleanedUpDesiredUnstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if foundDesired {
		if err := unstructured.SetNestedField(cleanedUpDesiredUnstructured.Object, desiredSpecTemplateSpec, "spec", "template", "spec"); err != nil {
			return false, "", pkgerrors.Errorf("could not write spec.template.spec to desired object for comparison")
		}
	}

	return compare.Diff(cleanedUpPatchedUnstructured, cleanedUpDesiredUnstructured)
}

type templateObjects struct {
	CurrentInfraMachineTemplate    *unstructured.Unstructured
	DesiredInfraMachineTemplate    *unstructured.Unstructured
	CurrentBootstrapConfigTemplate *unstructured.Unstructured
	DesiredBootstrapConfigTemplate *unstructured.Unstructured
}

func (p *rolloutPlanner) getTemplateObjects(ctx context.Context, oldMS, newMS *clusterv1.MachineSet) (*templateObjects, error) {
	templateObjects := &templateObjects{}
	var err error

	templateObjects.CurrentInfraMachineTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, oldMS.Spec.Template.Spec.InfrastructureRef, oldMS.Namespace)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(oldMS.Spec.Template.Spec.InfrastructureRef.Kind, "InfrastructureMachineTemplate"), oldMS.Name)
	}
	templateObjects.DesiredInfraMachineTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, newMS.Spec.Template.Spec.InfrastructureRef, newMS.Namespace)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(newMS.Spec.Template.Spec.InfrastructureRef.Kind, "InfrastructureMachineTemplate"), newMS.Name)
	}

	if oldMS.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		templateObjects.CurrentBootstrapConfigTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, oldMS.Spec.Template.Spec.Bootstrap.ConfigRef, oldMS.Namespace)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(oldMS.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "BootstrapConfigTemplate"), oldMS.Name)
		}
	}
	if newMS.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		templateObjects.DesiredBootstrapConfigTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, newMS.Spec.Template.Spec.Bootstrap.ConfigRef, newMS.Namespace)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(newMS.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "BootstrapConfigTemplate"), newMS.Name)
		}
	}

	return templateObjects, nil
}
