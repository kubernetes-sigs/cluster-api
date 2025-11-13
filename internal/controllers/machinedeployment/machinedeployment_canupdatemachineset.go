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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/internal/util/patch"
)

func (p *rolloutPlanner) canUpdateMachineSetInPlace(ctx context.Context, oldMS, newMS *clusterv1.MachineSet) (bool, error) {
	if p.overrideCanUpdateMachineSetInPlace != nil {
		return p.overrideCanUpdateMachineSetInPlace(ctx, oldMS, newMS)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("MachineSet", klog.KObj(oldMS))

	templateObjects, err := p.getTemplateObjects(ctx, oldMS, newMS)
	if err != nil {
		return false, err
	}

	// MachineSet cannot be updated in-place if the getTemplateObjects func was not able to get all InfraMachineTemplates.
	if templateObjects.CurrentInfraMachineTemplate == nil ||
		templateObjects.DesiredInfraMachineTemplate == nil {
		return false, nil
	}
	// MachineSet cannot be updated in-place if the BootstrapConfigTemplate is set on the oldMS but not on the newMS or vice versa.
	if (templateObjects.CurrentBootstrapConfigTemplate == nil && templateObjects.DesiredBootstrapConfigTemplate != nil) ||
		(templateObjects.CurrentBootstrapConfigTemplate != nil && templateObjects.DesiredBootstrapConfigTemplate == nil) {
		return false, nil
	}

	extensionHandlers, err := p.RuntimeClient.GetAllExtensions(ctx, runtimehooksv1.CanUpdateMachineSet, oldMS)
	if err != nil {
		return false, err
	}
	// MachineSet cannot be updated in-place if no CanUpdateMachineSet extensions are registered.
	if len(extensionHandlers) == 0 {
		return false, nil
	}
	if len(extensionHandlers) > 1 {
		return false, errors.Errorf("found multiple CanUpdateMachineSet hooks (%s): only one hook is supported", strings.Join(extensionHandlers, ","))
	}

	canUpdateMachineSet, reasons, err := p.canExtensionsUpdateMachineSet(ctx, oldMS, newMS, templateObjects, extensionHandlers)
	if err != nil {
		return false, err
	}
	if !canUpdateMachineSet {
		log.Info(fmt.Sprintf("MachineSet %s cannot be updated in-place by extensions", oldMS.Name), "reason", strings.Join(reasons, ","))
		return false, nil
	}
	log.Info(fmt.Sprintf("MachineSet %s can be updated in-place by extensions", oldMS.Name))
	return true, nil
}

// canExtensionsUpdateMachineSet calls CanUpdateMachineSet extensions to decide if a MachineSet can be updated in-place.
// Note: This is following the same general structure that is used in the Apply func in
// internal/controllers/topology/cluster/patches/engine.go.
func (p *rolloutPlanner) canExtensionsUpdateMachineSet(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (bool, []string, error) {
	if p.overrideCanExtensionsUpdateMachineSet != nil {
		return p.overrideCanExtensionsUpdateMachineSet(ctx, oldMS, newMS, templateObjects, extensionHandlers)
	}

	log := ctrl.LoggerFrom(ctx)

	// Create the CanUpdateMachineSet request.
	req, err := createRequest(oldMS, newMS, templateObjects)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to generate CanUpdateMachineSet request")
	}

	var reasons []string
	for _, extensionHandler := range extensionHandlers {
		// Call CanUpdateMachineSet extension.
		resp := &runtimehooksv1.CanUpdateMachineSetResponse{}
		if err := p.RuntimeClient.CallExtension(ctx, runtimehooksv1.CanUpdateMachineSet, oldMS, extensionHandler, req, resp); err != nil {
			return false, nil, err
		}

		// Apply patches from the CanUpdateMachineSet response to the request.
		if err := applyPatchesToRequest(ctx, req, resp); err != nil {
			return false, nil, errors.Wrapf(err, "failed to apply patches from extension %s to the CanUpdateMachineSet request", extensionHandler)
		}

		// Check if current and desired objects are now matching.
		var matches bool
		matches, reasons, err = matchesMachineSet(req)
		if err != nil {
			return false, nil, errors.Wrapf(err, "failed to compare current and desired objects after calling extension %s", extensionHandler)
		}
		if matches {
			return true, nil, nil
		}
		log.V(5).Info(fmt.Sprintf("MachineSet cannot be updated in-place yet after calling extension %s: %s", extensionHandler, strings.Join(reasons, ",")), "MachineSet", klog.KObj(oldMS))
	}

	return false, reasons, nil
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

func cleanupMachineSet(machine *clusterv1.MachineSet) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		// Set GVK because object is later marshalled with json.Marshal.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        machine.Name,
			Namespace:   machine.Namespace,
			Labels:      machine.Labels,
			Annotations: machine.Annotations,
		},
		Spec: *machine.Spec.DeepCopy(),
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
		if err := patch.ApplyPatchToTypedObject(ctx, &req.Current.MachineSet, resp.MachineSetPatch, "spec.template.spec"); err != nil {
			return err
		}
	}

	if resp.BootstrapConfigTemplatePatch.IsDefined() && req.Current.BootstrapConfigTemplate.Object != nil {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.BootstrapConfigTemplate, resp.BootstrapConfigTemplatePatch, "spec.template.spec"); err != nil {
			return err
		}
	}

	if resp.InfrastructureMachineTemplatePatch.IsDefined() {
		if _, err := patch.ApplyPatchToObject(ctx, &req.Current.InfrastructureMachineTemplate, resp.InfrastructureMachineTemplatePatch, "spec.template.spec"); err != nil {
			return err
		}
	}

	return nil
}

func matchesMachineSet(req *runtimehooksv1.CanUpdateMachineSetRequest) (bool, []string, error) {
	var reasons []string
	match, diff, err := matchesMachineSetSpec(&req.Current.MachineSet, &req.Desired.MachineSet)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to match MachineSet")
	}
	if !match {
		reasons = append(reasons, fmt.Sprintf("MachineSet cannot be updated in-place: %s", diff))
	}
	if req.Current.BootstrapConfigTemplate.Object != nil && req.Desired.BootstrapConfigTemplate.Object != nil {
		match, diff, err = matchesUnstructuredSpec(req.Current.BootstrapConfigTemplate, req.Desired.BootstrapConfigTemplate)
		if err != nil {
			return false, nil, errors.Wrapf(err, "failed to match %s", req.Current.BootstrapConfigTemplate.Object.GetObjectKind().GroupVersionKind().Kind)
		}
		if !match {
			reasons = append(reasons, fmt.Sprintf("%s cannot be updated in-place: %s", req.Current.BootstrapConfigTemplate.Object.GetObjectKind().GroupVersionKind().Kind, diff))
		}
	}
	match, diff, err = matchesUnstructuredSpec(req.Current.InfrastructureMachineTemplate, req.Desired.InfrastructureMachineTemplate)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to match %s", req.Current.InfrastructureMachineTemplate.Object.GetObjectKind().GroupVersionKind().Kind)
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
		return false, "", errors.Errorf("patched object is not an Unstructured")
	}
	desiredUnstructured, ok := desired.Object.(*unstructured.Unstructured)
	if !ok {
		return false, "", errors.Errorf("desired object is not an Unstructured")
	}

	// Note: Wrapping Unstructured spec.template.specs in an Unstructured for proper formatting of the diff.
	patchedSpecTemplateSpec, foundPatched, err := unstructured.NestedFieldNoCopy(patchedUnstructured.Object, "spec", "template", "spec")
	if err != nil {
		return false, "", errors.Errorf("could not read spec.template.spec from patched object")
	}
	desiredSpecTemplateSpec, foundDesired, err := unstructured.NestedFieldNoCopy(desiredUnstructured.Object, "spec", "template", "spec")
	if err != nil {
		return false, "", errors.Errorf("could not read spec.template.spec from desired object")
	}

	cleanedUpPatchedUnstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if foundPatched {
		if err := unstructured.SetNestedField(cleanedUpPatchedUnstructured.Object, patchedSpecTemplateSpec, "spec", "template", "spec"); err != nil {
			return false, "", errors.Errorf("could not write spec.template.spec to patched object for comparison")
		}
	}
	cleanedUpDesiredUnstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if foundDesired {
		if err := unstructured.SetNestedField(cleanedUpDesiredUnstructured.Object, desiredSpecTemplateSpec, "spec", "template", "spec"); err != nil {
			return false, "", errors.Errorf("could not write spec.template.spec to desired object for comparison")
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
		return nil, errors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(oldMS.Spec.Template.Spec.InfrastructureRef.Kind, "InfrastructureMachineTemplate"), oldMS.Name)
	}
	templateObjects.DesiredInfraMachineTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, newMS.Spec.Template.Spec.InfrastructureRef, newMS.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(newMS.Spec.Template.Spec.InfrastructureRef.Kind, "InfrastructureMachineTemplate"), newMS.Name)
	}

	if oldMS.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		templateObjects.CurrentBootstrapConfigTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, oldMS.Spec.Template.Spec.Bootstrap.ConfigRef, oldMS.Namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(oldMS.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "BootstrapConfigTemplate"), oldMS.Name)
		}
	}
	if newMS.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		templateObjects.DesiredBootstrapConfigTemplate, err = external.GetObjectFromContractVersionedRef(ctx, p.Client, newMS.Spec.Template.Spec.Bootstrap.ConfigRef, newMS.Namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s from MachineSet %s", cmp.Or(newMS.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "BootstrapConfigTemplate"), newMS.Name)
		}
	}

	return templateObjects, nil
}
