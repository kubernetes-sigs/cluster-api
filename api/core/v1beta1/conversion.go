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

package v1beta1

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"unsafe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

var apiVersionGetter = func(_ schema.GroupKind) (string, error) {
	return "", errors.New("apiVersionGetter not set")
}

func SetAPIVersionGetter(f func(gk schema.GroupKind) (string, error)) {
	apiVersionGetter = f
}

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Cluster)

	if err := Convert_v1beta1_Cluster_To_v1beta2_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef != nil {
		infraRef, err := convertToContractVersionedObjectReference(src.Spec.InfrastructureRef)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef != nil {
		controlPlaneRef, err := convertToContractVersionedObjectReference(src.Spec.ControlPlaneRef)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	restored := &clusterv1.Cluster{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	dst.Spec.Topology.ControlPlane.HealthCheck.Checks.UnhealthyMachineConditions = restored.Spec.Topology.ControlPlane.HealthCheck.Checks.UnhealthyMachineConditions
	for i, md := range restored.Spec.Topology.Workers.MachineDeployments {
		dst.Spec.Topology.Workers.MachineDeployments[i].HealthCheck.Checks.UnhealthyMachineConditions = md.HealthCheck.Checks.UnhealthyMachineConditions
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	initialization := clusterv1.ClusterInitializationStatus{}
	restoredControlPlaneInitialized := restored.Status.Initialization.ControlPlaneInitialized
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.ControlPlaneReady, ok, restoredControlPlaneInitialized, &initialization.ControlPlaneInitialized)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.ClusterInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}
	return nil
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Cluster)
	if err := Convert_v1beta2_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(src.Spec.InfrastructureRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.InfrastructureRef = infraRef
	}

	if src.Spec.ControlPlaneRef.IsDefined() {
		controlPlaneRef, err := convertToObjectReference(src.Spec.ControlPlaneRef, src.Namespace)
		if err != nil {
			return err
		}
		dst.Spec.ControlPlaneRef = controlPlaneRef
	}

	if dst.Spec.ClusterNetwork != nil && dst.Spec.ClusterNetwork.APIServerPort != nil &&
		*dst.Spec.ClusterNetwork.APIServerPort == 0 {
		dst.Spec.ClusterNetwork.APIServerPort = nil
	}

	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.ControlPlane.MachineHealthCheck != nil && dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate != nil {
			dst.Spec.Topology.ControlPlane.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
		}
		if dst.Spec.Topology.Workers != nil {
			for _, md := range dst.Spec.Topology.Workers.MachineDeployments {
				if md.MachineHealthCheck != nil && md.MachineHealthCheck.RemediationTemplate != nil {
					md.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
				}
			}
		}
	}

	dropEmptyStringsCluster(dst)

	return utilconversion.MarshalData(src, dst)
}

func (src *ClusterClass) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterClass)

	if err := Convert_v1beta1_ClusterClass_To_v1beta2_ClusterClass(src, dst, nil); err != nil {
		return err
	}

	restored := &clusterv1.ClusterClass{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	dst.Spec.ControlPlane.HealthCheck.Checks.UnhealthyMachineConditions = restored.Spec.ControlPlane.HealthCheck.Checks.UnhealthyMachineConditions
	for i, md := range restored.Spec.Workers.MachineDeployments {
		dst.Spec.Workers.MachineDeployments[i].HealthCheck.Checks.UnhealthyMachineConditions = md.HealthCheck.Checks.UnhealthyMachineConditions
	}

	// Recover intent for bool values converted to *bool.
	for i, patch := range dst.Spec.Patches {
		for j, definition := range patch.Definitions {
			var srcDefinition = &PatchDefinition{}
			for _, p := range src.Spec.Patches {
				if p.Name == patch.Name {
					if len(p.Definitions) == len(patch.Definitions) {
						srcDefinition = &p.Definitions[j]
					}
					break
				}
			}
			if srcDefinition == nil {
				return fmt.Errorf("definition %d for patch %s not found in source data", j, patch.Name)
			}
			var restoredPatchMatchControlPlane, restoredPatchMatchInfrastructureCluster *bool
			for _, p := range restored.Spec.Patches {
				if p.Name == patch.Name {
					if len(p.Definitions) == len(patch.Definitions) {
						restoredPatchMatchInfrastructureCluster = p.Definitions[j].Selector.MatchResources.InfrastructureCluster
						restoredPatchMatchControlPlane = p.Definitions[j].Selector.MatchResources.ControlPlane
					}
					break
				}
			}
			clusterv1.Convert_bool_To_Pointer_bool(srcDefinition.Selector.MatchResources.InfrastructureCluster, ok, restoredPatchMatchInfrastructureCluster, &definition.Selector.MatchResources.InfrastructureCluster)
			clusterv1.Convert_bool_To_Pointer_bool(srcDefinition.Selector.MatchResources.ControlPlane, ok, restoredPatchMatchControlPlane, &definition.Selector.MatchResources.ControlPlane)
			dst.Spec.Patches[i].Definitions[j] = definition
		}
	}

	for i, variable := range dst.Spec.Variables {
		var srcVariable *ClusterClassVariable
		for _, v := range src.Spec.Variables {
			if v.Name == variable.Name {
				srcVariable = &v
				break
			}
		}
		if srcVariable == nil {
			return fmt.Errorf("variable %q not found in source data", variable.Name)
		}
		var restoredVariableOpenAPIV3Schema *clusterv1.JSONSchemaProps
		for _, v := range restored.Spec.Variables {
			if v.Name == variable.Name {
				restoredVariableOpenAPIV3Schema = &v.Schema.OpenAPIV3Schema
				break
			}
		}
		if err := restoreBoolIntentJSONSchemaProps(&srcVariable.Schema.OpenAPIV3Schema, &variable.Schema.OpenAPIV3Schema, ok, restoredVariableOpenAPIV3Schema); err != nil {
			return err
		}
		dst.Spec.Variables[i] = variable
	}

	for i, variable := range dst.Status.Variables {
		var srcVariable *ClusterClassStatusVariable
		for _, v := range src.Status.Variables {
			if v.Name == variable.Name {
				srcVariable = &v
				break
			}
		}
		if srcVariable == nil {
			return fmt.Errorf("variable %q not found in source data", variable.Name)
		}
		var restoredVariable *clusterv1.ClusterClassStatusVariable
		var restoredVariableDefinitionsConflict *bool
		for _, v := range restored.Status.Variables {
			if v.Name == variable.Name {
				restoredVariable = &v
				restoredVariableDefinitionsConflict = v.DefinitionsConflict
				break
			}
		}
		clusterv1.Convert_bool_To_Pointer_bool(srcVariable.DefinitionsConflict, ok, restoredVariableDefinitionsConflict, &variable.DefinitionsConflict)

		for j, definition := range variable.Definitions {
			var srcDefinition *ClusterClassStatusVariableDefinition
			for _, d := range srcVariable.Definitions {
				if d.From == definition.From {
					srcDefinition = &d
				}
			}
			if srcDefinition == nil {
				return fmt.Errorf("definition %d for variable %s not found in source data", j, variable.Name)
			}
			var restoredVariableOpenAPIV3Schema *clusterv1.JSONSchemaProps
			if restoredVariable != nil {
				for _, d := range restoredVariable.Definitions {
					if d.From == definition.From {
						restoredVariableOpenAPIV3Schema = &d.Schema.OpenAPIV3Schema
					}
				}
			}
			if err := restoreBoolIntentJSONSchemaProps(&srcDefinition.Schema.OpenAPIV3Schema, &definition.Schema.OpenAPIV3Schema, ok, restoredVariableOpenAPIV3Schema); err != nil {
				return err
			}
			variable.Definitions[j] = definition
		}
		dst.Status.Variables[i] = variable
	}

	dst.Spec.KubernetesVersions = restored.Spec.KubernetesVersions

	dst.Spec.Upgrade.External.GenerateUpgradePlanExtension = restored.Spec.Upgrade.External.GenerateUpgradePlanExtension

	return nil
}

func restoreBoolIntentJSONSchemaProps(src *JSONSchemaProps, dst *clusterv1.JSONSchemaProps, hasRestored bool, restored *clusterv1.JSONSchemaProps) error {
	var restoredUniqueItems, restoreExclusiveMaximum, restoredExclusiveMinimum, restoreXPreserveUnknownFields, restoredXIntOrString *bool
	if restored != nil {
		restoredUniqueItems = restored.UniqueItems
		restoreExclusiveMaximum = restored.ExclusiveMaximum
		restoredExclusiveMinimum = restored.ExclusiveMinimum
		restoreXPreserveUnknownFields = restored.XPreserveUnknownFields
		restoredXIntOrString = restored.XIntOrString
	}
	clusterv1.Convert_bool_To_Pointer_bool(src.UniqueItems, hasRestored, restoredUniqueItems, &dst.UniqueItems)
	clusterv1.Convert_bool_To_Pointer_bool(src.ExclusiveMaximum, hasRestored, restoreExclusiveMaximum, &dst.ExclusiveMaximum)
	clusterv1.Convert_bool_To_Pointer_bool(src.ExclusiveMinimum, hasRestored, restoredExclusiveMinimum, &dst.ExclusiveMinimum)
	clusterv1.Convert_bool_To_Pointer_bool(src.XPreserveUnknownFields, hasRestored, restoreXPreserveUnknownFields, &dst.XPreserveUnknownFields)
	clusterv1.Convert_bool_To_Pointer_bool(src.XIntOrString, hasRestored, restoredXIntOrString, &dst.XIntOrString)

	for name, property := range dst.Properties {
		srcProperty, ok := src.Properties[name]
		if !ok {
			return fmt.Errorf("property %s not found in source data", name)
		}
		var restoredPropertyOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil {
			if v, ok := restored.Properties[name]; ok {
				restoredPropertyOpenAPIV3Schema = &v
			}
		}
		if err := restoreBoolIntentJSONSchemaProps(&srcProperty, &property, hasRestored, restoredPropertyOpenAPIV3Schema); err != nil {
			return err
		}
		dst.Properties[name] = property
	}
	if src.AdditionalProperties != nil {
		var restoredAdditionalPropertiesOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil {
			restoredAdditionalPropertiesOpenAPIV3Schema = restored.AdditionalProperties
		}
		if err := restoreBoolIntentJSONSchemaProps(src.AdditionalProperties, dst.AdditionalProperties, hasRestored, restoredAdditionalPropertiesOpenAPIV3Schema); err != nil {
			return err
		}
	}
	if src.Items != nil {
		var restoreItemsOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil {
			restoreItemsOpenAPIV3Schema = restored.Items
		}
		if err := restoreBoolIntentJSONSchemaProps(src.Items, dst.Items, hasRestored, restoreItemsOpenAPIV3Schema); err != nil {
			return err
		}
	}
	for i, value := range dst.AllOf {
		srcValue := src.AllOf[i]
		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil && len(src.AllOf) == len(dst.AllOf) {
			restoredValueOpenAPIV3Schema = &restored.AllOf[i]
		}
		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
			return err
		}
		dst.AllOf[i] = value
	}
	for i, value := range dst.OneOf {
		srcValue := src.OneOf[i]
		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil && len(src.OneOf) == len(dst.OneOf) {
			restoredValueOpenAPIV3Schema = &restored.OneOf[i]
		}
		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
			return err
		}
		dst.OneOf[i] = value
	}
	for i, value := range dst.AnyOf {
		srcValue := src.AnyOf[i]
		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil && len(src.AnyOf) == len(dst.AnyOf) {
			restoredValueOpenAPIV3Schema = &restored.AnyOf[i]
		}
		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
			return err
		}
		dst.AnyOf[i] = value
	}
	if src.Not != nil {
		var restoredNotOpenAPIV3Schema *clusterv1.JSONSchemaProps
		if restored != nil {
			restoredNotOpenAPIV3Schema = restored.Not
		}
		if err := restoreBoolIntentJSONSchemaProps(src.Not, dst.Not, hasRestored, restoredNotOpenAPIV3Schema); err != nil {
			return err
		}
	}
	return nil
}

func (dst *ClusterClass) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterClass)
	if err := Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.ControlPlane.MachineHealthCheck != nil && dst.Spec.ControlPlane.MachineHealthCheck.RemediationTemplate != nil {
		dst.Spec.ControlPlane.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
	}
	for _, md := range dst.Spec.Workers.MachineDeployments {
		if md.MachineHealthCheck != nil && md.MachineHealthCheck.RemediationTemplate != nil {
			md.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
		}
	}
	dropEmptyStringsClusterClass(dst)

	return utilconversion.MarshalData(src, dst)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Machine)

	if err := Convert_v1beta1_Machine_To_v1beta2_Machine(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec, &dst.Spec); err != nil {
		return err
	}

	restored := &clusterv1.Machine{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := clusterv1.MachineInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.MachineInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	// Recover other values.
	if ok {
		dst.Spec.MinReadySeconds = restored.Spec.MinReadySeconds
		dst.Spec.Taints = restored.Spec.Taints
		// Restore the phase, this also means that any client using v1beta1 during a round-trip
		// won't be able to write the Phase field. But that's okay as the only client writing the Phase
		// field should be the Machine controller.
		dst.Status.Phase = restored.Status.Phase
	}

	return nil
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Machine)

	if err := Convert_v1beta2_Machine_To_v1beta1_Machine(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(&src.Spec, &dst.Spec, src.Namespace); err != nil {
		return err
	}

	dropEmptyStringsMachineSpec(&dst.Spec)

	return utilconversion.MarshalData(src, dst)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSet)

	if err := Convert_v1beta1_MachineSet_To_v1beta2_MachineSet(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	if src.Spec.MinReadySeconds == 0 {
		dst.Spec.Template.Spec.MinReadySeconds = nil
	} else {
		dst.Spec.Template.Spec.MinReadySeconds = &src.Spec.MinReadySeconds
	}

	restored := &clusterv1.MachineSet{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover other values
	if ok {
		dst.Spec.Template.Spec.Taints = restored.Spec.Template.Spec.Taints
	}

	return nil
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSet)

	if err := Convert_v1beta2_MachineSet_To_v1beta1_MachineSet(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = ptr.Deref(src.Spec.Template.Spec.MinReadySeconds, 0)

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return utilconversion.MarshalData(src, dst)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1beta1_MachineDeployment_To_v1beta2_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	restored := &clusterv1.MachineDeployment{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	// Recover other values
	if ok {
		dst.Spec.Template.Spec.Taints = restored.Spec.Template.Spec.Taints
	}

	return nil
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeployment)

	if err := Convert_v1beta2_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return utilconversion.MarshalData(src, dst)
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheck)

	if err := Convert_v1beta1_MachineHealthCheck_To_v1beta2_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.MachineHealthCheck{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	dst.Spec.Checks.UnhealthyMachineConditions = restored.Spec.Checks.UnhealthyMachineConditions

	clusterv1.Convert_int32_To_Pointer_int32(src.Status.ExpectedMachines, ok, restored.Status.ExpectedMachines, &dst.Status.ExpectedMachines)
	clusterv1.Convert_int32_To_Pointer_int32(src.Status.CurrentHealthy, ok, restored.Status.CurrentHealthy, &dst.Status.CurrentHealthy)
	clusterv1.Convert_int32_To_Pointer_int32(src.Status.RemediationsAllowed, ok, restored.Status.RemediationsAllowed, &dst.Status.RemediationsAllowed)

	return nil
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheck)
	if err := Convert_v1beta2_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.RemediationTemplate != nil {
		dst.Spec.RemediationTemplate.Namespace = src.Namespace
	}

	return utilconversion.MarshalData(src, dst)
}

func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachinePool)

	if err := Convert_v1beta1_MachinePool_To_v1beta2_MachinePool(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	restored := &clusterv1.MachinePool{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := clusterv1.MachinePoolInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.MachinePoolInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	// Recover other values
	if ok {
		dst.Spec.Template.Spec.Taints = restored.Spec.Template.Spec.Taints
	}

	return nil
}

func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachinePool)

	if err := Convert_v1beta2_MachinePool_To_v1beta1_MachinePool(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return utilconversion.MarshalData(src, dst)
}

func (src *MachineDrainRule) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDrainRule)
	return Convert_v1beta1_MachineDrainRule_To_v1beta2_MachineDrainRule(src, dst, nil)
}

func (dst *MachineDrainRule) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDrainRule)
	return Convert_v1beta2_MachineDrainRule_To_v1beta1_MachineDrainRule(src, dst, nil)
}

func Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(in *clusterv1.ClusterClass, out *ClusterClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(in, out, s); err != nil {
		return err
	}

	if out.Spec.Infrastructure.Ref != nil {
		out.Spec.Infrastructure.Ref.Namespace = in.Namespace
	}
	if out.Spec.ControlPlane.Ref != nil {
		out.Spec.ControlPlane.Ref.Namespace = in.Namespace
	}
	if out.Spec.ControlPlane.MachineInfrastructure != nil && out.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		out.Spec.ControlPlane.MachineInfrastructure.Ref.Namespace = in.Namespace
	}
	for _, md := range out.Spec.Workers.MachineDeployments {
		if md.Template.Bootstrap.Ref != nil {
			md.Template.Bootstrap.Ref.Namespace = in.Namespace
		}
		if md.Template.Infrastructure.Ref != nil {
			md.Template.Infrastructure.Ref.Namespace = in.Namespace
		}
	}
	for _, mp := range out.Spec.Workers.MachinePools {
		if mp.Template.Bootstrap.Ref != nil {
			mp.Template.Bootstrap.Ref.Namespace = in.Namespace
		}
		if mp.Template.Infrastructure.Ref != nil {
			mp.Template.Infrastructure.Ref.Namespace = in.Namespace
		}
	}
	return nil
}

func Convert_v1beta2_ClusterClassSpec_To_v1beta1_ClusterClassSpec(in *clusterv1.ClusterClassSpec, out *ClusterClassSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassSpec_To_v1beta1_ClusterClassSpec(in, out, s); err != nil {
		return err
	}

	if in.Infrastructure.Naming.Template != "" {
		out.InfrastructureNamingStrategy = &InfrastructureNamingStrategy{
			Template: ptr.To(in.Infrastructure.Naming.Template),
		}
	}

	return nil
}

func Convert_v1beta2_InfrastructureClass_To_v1beta1_LocalObjectTemplate(in *clusterv1.InfrastructureClass, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	if in == nil {
		return nil
	}

	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta1_ClusterClassSpec_To_v1beta2_ClusterClassSpec(in *ClusterClassSpec, out *clusterv1.ClusterClassSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassSpec_To_v1beta2_ClusterClassSpec(in, out, s); err != nil {
		return err
	}

	if in.InfrastructureNamingStrategy != nil {
		out.Infrastructure.Naming = clusterv1.InfrastructureClassNamingSpec{
			Template: ptr.Deref(in.InfrastructureNamingStrategy.Template, ""),
		}
	}
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_InfrastructureClass(in *LocalObjectTemplate, out *clusterv1.InfrastructureClass, s apimachineryconversion.Scope) error {
	if in == nil {
		return nil
	}

	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func Convert_v1beta1_ControlPlaneClass_To_v1beta2_ControlPlaneClass(in *ControlPlaneClass, out *clusterv1.ControlPlaneClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ControlPlaneClass_To_v1beta2_ControlPlaneClass(in, out, s); err != nil {
		return err
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(&in.LocalObjectTemplate, &out.TemplateRef, s)

	if in.MachineHealthCheck != nil {
		for _, c := range in.MachineHealthCheck.UnhealthyConditions {
			out.HealthCheck.Checks.UnhealthyNodeConditions = append(out.HealthCheck.Checks.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
				Type:           c.Type,
				Status:         c.Status,
				TimeoutSeconds: clusterv1.ConvertToSeconds(&c.Timeout),
			})
		}
		out.HealthCheck.Checks.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.MachineHealthCheck.NodeStartupTimeout)
		out.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = in.MachineHealthCheck.MaxUnhealthy
		out.HealthCheck.Remediation.TriggerIf.UnhealthyInRange = ptr.Deref(in.MachineHealthCheck.UnhealthyRange, "")
		if in.MachineHealthCheck.RemediationTemplate != nil {
			if err := Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in.MachineHealthCheck.RemediationTemplate, &out.HealthCheck.Remediation.TemplateRef, s); err != nil {
				return err
			}
		}
	}
	if in.NamingStrategy != nil {
		out.Naming = clusterv1.ControlPlaneClassNamingSpec{
			Template: ptr.Deref(in.NamingStrategy.Template, ""),
		}
	}
	if in.MachineInfrastructure != nil {
		if err := Convert_v1beta1_LocalObjectTemplate_To_v1beta2_ControlPlaneClassMachineInfrastructureTemplate(in.MachineInfrastructure, &out.MachineInfrastructure, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_ControlPlaneClass_To_v1beta1_ControlPlaneClass(in *clusterv1.ControlPlaneClass, out *ControlPlaneClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ControlPlaneClass_To_v1beta1_ControlPlaneClass(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, &out.LocalObjectTemplate, s)

	if !reflect.DeepEqual(in.HealthCheck, clusterv1.ControlPlaneClassHealthCheck{}) {
		out.MachineHealthCheck = &MachineHealthCheckClass{}
		for _, c := range in.HealthCheck.Checks.UnhealthyNodeConditions {
			out.MachineHealthCheck.UnhealthyConditions = append(out.MachineHealthCheck.UnhealthyConditions, UnhealthyCondition{
				Type:    c.Type,
				Status:  c.Status,
				Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(c.TimeoutSeconds), metav1.Duration{}),
			})
		}
		out.MachineHealthCheck.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.HealthCheck.Checks.NodeStartupTimeoutSeconds)
		out.MachineHealthCheck.MaxUnhealthy = in.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo
		if in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange != "" {
			out.MachineHealthCheck.UnhealthyRange = ptr.To(in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange)
		}
		if in.HealthCheck.Remediation.TemplateRef.IsDefined() {
			out.MachineHealthCheck.RemediationTemplate = &corev1.ObjectReference{}
			if err := Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(&in.HealthCheck.Remediation.TemplateRef, out.MachineHealthCheck.RemediationTemplate, s); err != nil {
				return err
			}
		}
	}
	if in.Naming.Template != "" {
		out.NamingStrategy = &ControlPlaneClassNamingStrategy{
			Template: ptr.To(in.Naming.Template),
		}
	}
	if in.MachineInfrastructure.TemplateRef.IsDefined() {
		out.MachineInfrastructure = &LocalObjectTemplate{}
		if err := Convert_v1beta2_ControlPlaneClassMachineInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(&in.MachineInfrastructure, out.MachineInfrastructure, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta1_ControlPlaneTopology_To_v1beta2_ControlPlaneTopology(in *ControlPlaneTopology, out *clusterv1.ControlPlaneTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ControlPlaneTopology_To_v1beta2_ControlPlaneTopology(in, out, s); err != nil {
		return err
	}
	if in.Variables != nil {
		if err := autoConvert_v1beta1_ControlPlaneVariables_To_v1beta2_ControlPlaneVariables(in.Variables, &out.Variables, s); err != nil {
			return err
		}
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)

	if in.MachineHealthCheck != nil {
		out.HealthCheck.Enabled = in.MachineHealthCheck.Enable
		for _, c := range in.MachineHealthCheck.UnhealthyConditions {
			out.HealthCheck.Checks.UnhealthyNodeConditions = append(out.HealthCheck.Checks.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
				Type:           c.Type,
				Status:         c.Status,
				TimeoutSeconds: clusterv1.ConvertToSeconds(&c.Timeout),
			})
		}
		out.HealthCheck.Checks.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.MachineHealthCheck.NodeStartupTimeout)
		out.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = in.MachineHealthCheck.MaxUnhealthy
		out.HealthCheck.Remediation.TriggerIf.UnhealthyInRange = ptr.Deref(in.MachineHealthCheck.UnhealthyRange, "")
		if in.MachineHealthCheck.RemediationTemplate != nil {
			if err := Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in.MachineHealthCheck.RemediationTemplate, &out.HealthCheck.Remediation.TemplateRef, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1beta2_ControlPlaneTopology_To_v1beta1_ControlPlaneTopology(in *clusterv1.ControlPlaneTopology, out *ControlPlaneTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ControlPlaneTopology_To_v1beta1_ControlPlaneTopology(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Variables, clusterv1.ControlPlaneVariables{}) {
		out.Variables = &ControlPlaneVariables{}
		if err := autoConvert_v1beta2_ControlPlaneVariables_To_v1beta1_ControlPlaneVariables(&in.Variables, out.Variables, s); err != nil {
			return err
		}
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)

	if !reflect.DeepEqual(in.HealthCheck, clusterv1.ControlPlaneTopologyHealthCheck{}) {
		out.MachineHealthCheck = &MachineHealthCheckTopology{}
		out.MachineHealthCheck.Enable = in.HealthCheck.Enabled
		for _, c := range in.HealthCheck.Checks.UnhealthyNodeConditions {
			out.MachineHealthCheck.UnhealthyConditions = append(out.MachineHealthCheck.UnhealthyConditions, UnhealthyCondition{
				Type:    c.Type,
				Status:  c.Status,
				Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(c.TimeoutSeconds), metav1.Duration{}),
			})
		}
		out.MachineHealthCheck.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.HealthCheck.Checks.NodeStartupTimeoutSeconds)
		out.MachineHealthCheck.MaxUnhealthy = in.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo
		if in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange != "" {
			out.MachineHealthCheck.UnhealthyRange = ptr.To(in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange)
		}
		if in.HealthCheck.Remediation.TemplateRef.IsDefined() {
			out.MachineHealthCheck.RemediationTemplate = &corev1.ObjectReference{}
			if err := Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(&in.HealthCheck.Remediation.TemplateRef, out.MachineHealthCheck.RemediationTemplate, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1beta1_MachineDeploymentClass_To_v1beta2_MachineDeploymentClass(in *MachineDeploymentClass, out *clusterv1.MachineDeploymentClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentClass_To_v1beta2_MachineDeploymentClass(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(&in.Template.Metadata, &out.Metadata, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachineDeploymentClassBootstrapTemplate(&in.Template.Bootstrap, &out.Bootstrap, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachineDeploymentClassInfrastructureTemplate(&in.Template.Infrastructure, &out.Infrastructure, s); err != nil {
		return err
	}
	if in.Strategy != nil {
		out.Rollout.Strategy.Type = clusterv1.MachineDeploymentRolloutStrategyType(in.Strategy.Type)
		if in.Strategy.RollingUpdate != nil {
			out.Rollout.Strategy.RollingUpdate.MaxUnavailable = in.Strategy.RollingUpdate.MaxUnavailable
			out.Rollout.Strategy.RollingUpdate.MaxSurge = in.Strategy.RollingUpdate.MaxSurge
			if in.Strategy.RollingUpdate.DeletePolicy != nil {
				out.Deletion.Order = clusterv1.MachineSetDeletionOrder(*in.Strategy.RollingUpdate.DeletePolicy)
			}
		}
		if in.Strategy.Remediation != nil && in.Strategy.Remediation.MaxInFlight != nil {
			out.HealthCheck.Remediation.MaxInFlight = in.Strategy.Remediation.MaxInFlight
		}
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)

	if in.MachineHealthCheck != nil {
		for _, c := range in.MachineHealthCheck.UnhealthyConditions {
			out.HealthCheck.Checks.UnhealthyNodeConditions = append(out.HealthCheck.Checks.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
				Type:           c.Type,
				Status:         c.Status,
				TimeoutSeconds: clusterv1.ConvertToSeconds(&c.Timeout),
			})
		}
		out.HealthCheck.Checks.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.MachineHealthCheck.NodeStartupTimeout)
		out.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = in.MachineHealthCheck.MaxUnhealthy
		out.HealthCheck.Remediation.TriggerIf.UnhealthyInRange = ptr.Deref(in.MachineHealthCheck.UnhealthyRange, "")
		if in.MachineHealthCheck.RemediationTemplate != nil {
			if err := Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in.MachineHealthCheck.RemediationTemplate, &out.HealthCheck.Remediation.TemplateRef, s); err != nil {
				return err
			}
		}
	}
	if in.NamingStrategy != nil {
		out.Naming = clusterv1.MachineDeploymentClassNamingSpec{
			Template: ptr.Deref(in.NamingStrategy.Template, ""),
		}
	}
	return nil
}

func Convert_v1beta2_MachineDeploymentClass_To_v1beta1_MachineDeploymentClass(in *clusterv1.MachineDeploymentClass, out *MachineDeploymentClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentClass_To_v1beta1_MachineDeploymentClass(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(&in.Metadata, &out.Template.Metadata, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_MachineDeploymentClassBootstrapTemplate_To_v1beta1_LocalObjectTemplate(&in.Bootstrap, &out.Template.Bootstrap, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_MachineDeploymentClassInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(&in.Infrastructure, &out.Template.Infrastructure, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Rollout.Strategy, clusterv1.MachineDeploymentClassRolloutStrategy{}) {
		out.Strategy = &MachineDeploymentStrategy{}
		out.Strategy.Type = MachineDeploymentStrategyType(in.Rollout.Strategy.Type)
		if !reflect.DeepEqual(in.Rollout.Strategy.RollingUpdate, clusterv1.MachineDeploymentClassRolloutStrategyRollingUpdate{}) {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
			out.Strategy.RollingUpdate.MaxUnavailable = in.Rollout.Strategy.RollingUpdate.MaxUnavailable
			out.Strategy.RollingUpdate.MaxSurge = in.Rollout.Strategy.RollingUpdate.MaxSurge
		}
	}
	if in.Deletion.Order != "" {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.RollingUpdate == nil {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
		}
		out.Strategy.RollingUpdate.DeletePolicy = ptr.To(string(in.Deletion.Order))
	}
	if in.HealthCheck.Remediation.MaxInFlight != nil {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.Remediation == nil {
			out.Strategy.Remediation = &RemediationStrategy{}
		}
		out.Strategy.Remediation.MaxInFlight = in.HealthCheck.Remediation.MaxInFlight
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)

	// Check if HealthCheck is not nil and another field apart from MaxInFlight is set (MaxInFlight is set in Strategy above).
	if !reflect.DeepEqual(in.HealthCheck, clusterv1.MachineDeploymentClassHealthCheck{
		Remediation: clusterv1.MachineDeploymentClassHealthCheckRemediation{MaxInFlight: in.HealthCheck.Remediation.MaxInFlight},
	}) {
		out.MachineHealthCheck = &MachineHealthCheckClass{}
		for _, c := range in.HealthCheck.Checks.UnhealthyNodeConditions {
			out.MachineHealthCheck.UnhealthyConditions = append(out.MachineHealthCheck.UnhealthyConditions, UnhealthyCondition{
				Type:    c.Type,
				Status:  c.Status,
				Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(c.TimeoutSeconds), metav1.Duration{}),
			})
		}
		out.MachineHealthCheck.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.HealthCheck.Checks.NodeStartupTimeoutSeconds)
		out.MachineHealthCheck.MaxUnhealthy = in.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo
		if in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange != "" {
			out.MachineHealthCheck.UnhealthyRange = ptr.To(in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange)
		}
		if in.HealthCheck.Remediation.TemplateRef.IsDefined() {
			out.MachineHealthCheck.RemediationTemplate = &corev1.ObjectReference{}
			if err := Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(&in.HealthCheck.Remediation.TemplateRef, out.MachineHealthCheck.RemediationTemplate, s); err != nil {
				return err
			}
		}
	}
	if in.Naming.Template != "" {
		out.NamingStrategy = &MachineDeploymentClassNamingStrategy{
			Template: ptr.To(in.Naming.Template),
		}
	}
	return nil
}

func Convert_v1beta1_MachineDeploymentTopology_To_v1beta2_MachineDeploymentTopology(in *MachineDeploymentTopology, out *clusterv1.MachineDeploymentTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentTopology_To_v1beta2_MachineDeploymentTopology(in, out, s); err != nil {
		return err
	}
	if in.Variables != nil {
		if err := autoConvert_v1beta1_MachineDeploymentVariables_To_v1beta2_MachineDeploymentVariables(in.Variables, &out.Variables, s); err != nil {
			return err
		}
	}
	if in.Strategy != nil {
		out.Rollout.Strategy.Type = clusterv1.MachineDeploymentRolloutStrategyType(in.Strategy.Type)
		if in.Strategy.RollingUpdate != nil {
			out.Rollout.Strategy.RollingUpdate.MaxUnavailable = in.Strategy.RollingUpdate.MaxUnavailable
			out.Rollout.Strategy.RollingUpdate.MaxSurge = in.Strategy.RollingUpdate.MaxSurge
			if in.Strategy.RollingUpdate.DeletePolicy != nil {
				out.Deletion.Order = clusterv1.MachineSetDeletionOrder(*in.Strategy.RollingUpdate.DeletePolicy)
			}
		}
		if in.Strategy.Remediation != nil && in.Strategy.Remediation.MaxInFlight != nil {
			out.HealthCheck.Remediation.MaxInFlight = in.Strategy.Remediation.MaxInFlight
		}
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)

	if in.MachineHealthCheck != nil {
		out.HealthCheck.Enabled = in.MachineHealthCheck.Enable
		for _, c := range in.MachineHealthCheck.UnhealthyConditions {
			out.HealthCheck.Checks.UnhealthyNodeConditions = append(out.HealthCheck.Checks.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
				Type:           c.Type,
				Status:         c.Status,
				TimeoutSeconds: clusterv1.ConvertToSeconds(&c.Timeout),
			})
		}
		out.HealthCheck.Checks.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.MachineHealthCheck.NodeStartupTimeout)
		out.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = in.MachineHealthCheck.MaxUnhealthy
		out.HealthCheck.Remediation.TriggerIf.UnhealthyInRange = ptr.Deref(in.MachineHealthCheck.UnhealthyRange, "")
		if in.MachineHealthCheck.RemediationTemplate != nil {
			if err := Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in.MachineHealthCheck.RemediationTemplate, &out.HealthCheck.Remediation.TemplateRef, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1beta2_MachineDeploymentTopology_To_v1beta1_MachineDeploymentTopology(in *clusterv1.MachineDeploymentTopology, out *MachineDeploymentTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentTopology_To_v1beta1_MachineDeploymentTopology(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Variables, clusterv1.MachineDeploymentVariables{}) {
		out.Variables = &MachineDeploymentVariables{}
		if err := autoConvert_v1beta2_MachineDeploymentVariables_To_v1beta1_MachineDeploymentVariables(&in.Variables, out.Variables, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.Rollout.Strategy, clusterv1.MachineDeploymentTopologyRolloutStrategy{}) {
		out.Strategy = &MachineDeploymentStrategy{}
		out.Strategy.Type = MachineDeploymentStrategyType(in.Rollout.Strategy.Type)
		if !reflect.DeepEqual(in.Rollout.Strategy.RollingUpdate, clusterv1.MachineDeploymentTopologyRolloutStrategyRollingUpdate{}) {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
			out.Strategy.RollingUpdate.MaxUnavailable = in.Rollout.Strategy.RollingUpdate.MaxUnavailable
			out.Strategy.RollingUpdate.MaxSurge = in.Rollout.Strategy.RollingUpdate.MaxSurge
		}
	}
	if in.Deletion.Order != "" {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.RollingUpdate == nil {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
		}
		out.Strategy.RollingUpdate.DeletePolicy = ptr.To(string(in.Deletion.Order))
	}
	if in.HealthCheck.Remediation.MaxInFlight != nil {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.Remediation == nil {
			out.Strategy.Remediation = &RemediationStrategy{}
		}
		out.Strategy.Remediation.MaxInFlight = in.HealthCheck.Remediation.MaxInFlight
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)

	// Check if HealthCheck is not nil and another field apart from MaxInFlight is set (MaxInFlight is set in Strategy above).
	if !reflect.DeepEqual(in.HealthCheck, clusterv1.MachineDeploymentTopologyHealthCheck{
		Remediation: clusterv1.MachineDeploymentTopologyHealthCheckRemediation{MaxInFlight: in.HealthCheck.Remediation.MaxInFlight},
	}) {
		out.MachineHealthCheck = &MachineHealthCheckTopology{}
		out.MachineHealthCheck.Enable = in.HealthCheck.Enabled
		for _, c := range in.HealthCheck.Checks.UnhealthyNodeConditions {
			out.MachineHealthCheck.UnhealthyConditions = append(out.MachineHealthCheck.UnhealthyConditions, UnhealthyCondition{
				Type:    c.Type,
				Status:  c.Status,
				Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(c.TimeoutSeconds), metav1.Duration{}),
			})
		}
		out.MachineHealthCheck.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.HealthCheck.Checks.NodeStartupTimeoutSeconds)
		out.MachineHealthCheck.MaxUnhealthy = in.HealthCheck.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo
		if in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange != "" {
			out.MachineHealthCheck.UnhealthyRange = ptr.To(in.HealthCheck.Remediation.TriggerIf.UnhealthyInRange)
		}
		if in.HealthCheck.Remediation.TemplateRef.IsDefined() {
			out.MachineHealthCheck.RemediationTemplate = &corev1.ObjectReference{}
			if err := Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(&in.HealthCheck.Remediation.TemplateRef, out.MachineHealthCheck.RemediationTemplate, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func Convert_v1beta1_MachinePoolClass_To_v1beta2_MachinePoolClass(in *MachinePoolClass, out *clusterv1.MachinePoolClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolClass_To_v1beta2_MachinePoolClass(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(&in.Template.Metadata, &out.Metadata, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachinePoolClassBootstrapTemplate(&in.Template.Bootstrap, &out.Bootstrap, s); err != nil {
		return err
	}
	if err := Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachinePoolClassInfrastructureTemplate(&in.Template.Infrastructure, &out.Infrastructure, s); err != nil {
		return err
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	if in.NamingStrategy != nil {
		out.Naming = clusterv1.MachinePoolClassNamingSpec{
			Template: ptr.Deref(in.NamingStrategy.Template, ""),
		}
	}
	return nil
}

func Convert_v1beta2_MachinePoolClass_To_v1beta1_MachinePoolClass(in *clusterv1.MachinePoolClass, out *MachinePoolClass, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolClass_To_v1beta1_MachinePoolClass(in, out, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(&in.Metadata, &out.Template.Metadata, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_MachinePoolClassBootstrapTemplate_To_v1beta1_LocalObjectTemplate(&in.Bootstrap, &out.Template.Bootstrap, s); err != nil {
		return err
	}
	if err := Convert_v1beta2_MachinePoolClassInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(&in.Infrastructure, &out.Template.Infrastructure, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)
	if in.Naming.Template != "" {
		out.NamingStrategy = &MachinePoolClassNamingStrategy{
			Template: ptr.To(in.Naming.Template),
		}
	}
	return nil
}

func Convert_v1beta1_MachinePoolTopology_To_v1beta2_MachinePoolTopology(in *MachinePoolTopology, out *clusterv1.MachinePoolTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolTopology_To_v1beta2_MachinePoolTopology(in, out, s); err != nil {
		return err
	}
	if in.Variables != nil {
		if err := autoConvert_v1beta1_MachinePoolVariables_To_v1beta2_MachinePoolVariables(in.Variables, &out.Variables, s); err != nil {
			return err
		}
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_MachinePoolTopology_To_v1beta1_MachinePoolTopology(in *clusterv1.MachinePoolTopology, out *MachinePoolTopology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolTopology_To_v1beta1_MachinePoolTopology(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Variables, clusterv1.MachinePoolVariables{}) {
		out.Variables = &MachinePoolVariables{}
		if err := autoConvert_v1beta2_MachinePoolVariables_To_v1beta1_MachinePoolVariables(&in.Variables, out.Variables, s); err != nil {
			return err
		}
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta1_MachineSpec_To_v1beta2_MachineSpec(in *MachineSpec, out *clusterv1.MachineSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineSpec_To_v1beta2_MachineSpec(in, out, s); err != nil {
		return err
	}
	out.Deletion.NodeDrainTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDrainTimeout)
	out.Deletion.NodeVolumeDetachTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeVolumeDetachTimeout)
	out.Deletion.NodeDeletionTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeDeletionTimeout)
	return nil
}

func Convert_v1beta2_ClusterClassStatus_To_v1beta1_ClusterClassStatus(in *clusterv1.ClusterClassStatus, out *ClusterClassStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassStatus_To_v1beta1_ClusterClassStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &ClusterClassV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_ClusterClassStatus_To_v1beta2_ClusterClassStatus(in *ClusterClassStatus, out *clusterv1.ClusterClassStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassStatus_To_v1beta2_ClusterClassStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.ClusterClassDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.ClusterClassV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	return nil
}

func Convert_v1beta2_ClusterStatus_To_v1beta1_ClusterStatus(in *clusterv1.ClusterStatus, out *ClusterStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterStatus_To_v1beta1_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	out.ControlPlaneReady = ptr.Deref(in.Initialization.ControlPlaneInitialized, false)
	out.InfrastructureReady = ptr.Deref(in.Initialization.InfrastructureProvisioned, false)

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = FailureDomains{}
		for _, fd := range in.FailureDomains {
			out.FailureDomains[fd.Name] = FailureDomainSpec{
				ControlPlane: ptr.Deref(fd.ControlPlane, false),
				Attributes:   fd.Attributes,
			}
		}
	}

	// Move new conditions (v1beta2), controlPlane and workers counters to the v1beta2 field.
	if in.Conditions == nil && in.ControlPlane == nil && in.Workers == nil {
		return nil
	}
	out.V1Beta2 = &ClusterV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	if in.ControlPlane != nil {
		out.V1Beta2.ControlPlane = &ClusterControlPlaneStatus{
			DesiredReplicas:   in.ControlPlane.DesiredReplicas,
			Replicas:          in.ControlPlane.Replicas,
			UpToDateReplicas:  in.ControlPlane.UpToDateReplicas,
			ReadyReplicas:     in.ControlPlane.ReadyReplicas,
			AvailableReplicas: in.ControlPlane.AvailableReplicas,
		}
	}
	if in.Workers != nil {
		out.V1Beta2.Workers = &WorkersStatus{
			DesiredReplicas:   in.Workers.DesiredReplicas,
			Replicas:          in.Workers.Replicas,
			UpToDateReplicas:  in.Workers.UpToDateReplicas,
			ReadyReplicas:     in.Workers.ReadyReplicas,
			AvailableReplicas: in.Workers.AvailableReplicas,
		}
	}
	return nil
}

func Convert_v1beta1_Topology_To_v1beta2_Topology(in *Topology, out *clusterv1.Topology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_Topology_To_v1beta2_Topology(in, out, s); err != nil {
		return err
	}
	if in.Workers != nil {
		if err := autoConvert_v1beta1_WorkersTopology_To_v1beta2_WorkersTopology(in.Workers, &out.Workers, s); err != nil {
			return err
		}
	}
	out.ClassRef.Name = in.Class
	out.ClassRef.Namespace = in.ClassNamespace
	return nil
}

func Convert_v1beta1_ClusterStatus_To_v1beta2_ClusterStatus(in *ClusterStatus, out *clusterv1.ClusterStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterStatus_To_v1beta2_ClusterStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2), controlPlane and workers counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		if in.V1Beta2.ControlPlane != nil {
			out.ControlPlane = &clusterv1.ClusterControlPlaneStatus{
				DesiredReplicas:   in.V1Beta2.ControlPlane.DesiredReplicas,
				Replicas:          in.V1Beta2.ControlPlane.Replicas,
				UpToDateReplicas:  in.V1Beta2.ControlPlane.UpToDateReplicas,
				ReadyReplicas:     in.V1Beta2.ControlPlane.ReadyReplicas,
				AvailableReplicas: in.V1Beta2.ControlPlane.AvailableReplicas,
			}
		}
		if in.V1Beta2.Workers != nil {
			out.Workers = &clusterv1.WorkersStatus{
				DesiredReplicas:   in.V1Beta2.Workers.DesiredReplicas,
				Replicas:          in.V1Beta2.Workers.Replicas,
				UpToDateReplicas:  in.V1Beta2.Workers.UpToDateReplicas,
				ReadyReplicas:     in.V1Beta2.Workers.ReadyReplicas,
				AvailableReplicas: in.V1Beta2.Workers.AvailableReplicas,
			}
		}
	}

	// Move ControlPlaneReady and InfrastructureReady to Initialization is implemented in ConvertTo.

	// Move FailureDomains
	if in.FailureDomains != nil {
		out.FailureDomains = []clusterv1.FailureDomain{}
		domainNames := slices.Collect(maps.Keys(in.FailureDomains))
		sort.Strings(domainNames)
		for _, name := range domainNames {
			fd := in.FailureDomains[name]
			out.FailureDomains = append(out.FailureDomains, clusterv1.FailureDomain{
				Name:         name,
				ControlPlane: ptr.To(fd.ControlPlane),
				Attributes:   fd.Attributes,
			})
		}
	}

	// Move legacy conditions (v1beta1), FailureReason and FailureMessage to the deprecated field.
	if in.Conditions == nil && in.FailureReason == nil && in.FailureMessage == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_Topology_To_v1beta1_Topology(in *clusterv1.Topology, out *Topology, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Topology_To_v1beta1_Topology(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Workers, clusterv1.WorkersTopology{}) {
		out.Workers = &WorkersTopology{}
		if err := autoConvert_v1beta2_WorkersTopology_To_v1beta1_WorkersTopology(&in.Workers, out.Workers, s); err != nil {
			return err
		}
	}
	out.Class = in.ClassRef.Name
	out.ClassNamespace = in.ClassRef.Namespace
	return nil
}

func Convert_v1beta2_MachineDeploymentStatus_To_v1beta1_MachineDeploymentStatus(in *clusterv1.MachineDeploymentStatus, out *MachineDeploymentStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentStatus_To_v1beta1_MachineDeploymentStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.AvailableReplicas = 0
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1) and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
		out.UpdatedReplicas = in.Deprecated.V1Beta1.UpdatedReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
	}

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachineDeploymentV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachineDeploymentStatus_To_v1beta2_MachineDeploymentStatus(in *MachineDeploymentStatus, out *clusterv1.MachineDeploymentStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineDeploymentStatus_To_v1beta2_MachineDeploymentStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.AvailableReplicas = nil
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1) and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineDeploymentDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas
	out.Deprecated.V1Beta1.UpdatedReplicas = in.UpdatedReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	return nil
}

func Convert_v1beta1_MachineHealthCheckSpec_To_v1beta2_MachineHealthCheckSpec(in *MachineHealthCheckSpec, out *clusterv1.MachineHealthCheckSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineHealthCheckSpec_To_v1beta2_MachineHealthCheckSpec(in, out, s); err != nil {
		return err
	}

	for _, c := range in.UnhealthyConditions {
		out.Checks.UnhealthyNodeConditions = append(out.Checks.UnhealthyNodeConditions, clusterv1.UnhealthyNodeCondition{
			Type:           c.Type,
			Status:         c.Status,
			TimeoutSeconds: clusterv1.ConvertToSeconds(&c.Timeout),
		})
	}
	out.Checks.NodeStartupTimeoutSeconds = clusterv1.ConvertToSeconds(in.NodeStartupTimeout)
	out.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo = in.MaxUnhealthy
	out.Remediation.TriggerIf.UnhealthyInRange = ptr.Deref(in.UnhealthyRange, "")
	if in.RemediationTemplate != nil {
		if err := Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in.RemediationTemplate, &out.Remediation.TemplateRef, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta2_MachineHealthCheckSpec_To_v1beta1_MachineHealthCheckSpec(in *clusterv1.MachineHealthCheckSpec, out *MachineHealthCheckSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineHealthCheckSpec_To_v1beta1_MachineHealthCheckSpec(in, out, s); err != nil {
		return err
	}

	for _, c := range in.Checks.UnhealthyNodeConditions {
		out.UnhealthyConditions = append(out.UnhealthyConditions, UnhealthyCondition{
			Type:    c.Type,
			Status:  c.Status,
			Timeout: ptr.Deref(clusterv1.ConvertFromSeconds(c.TimeoutSeconds), metav1.Duration{}),
		})
	}
	out.NodeStartupTimeout = clusterv1.ConvertFromSeconds(in.Checks.NodeStartupTimeoutSeconds)
	out.MaxUnhealthy = in.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo
	if in.Remediation.TriggerIf.UnhealthyInRange != "" {
		out.UnhealthyRange = ptr.To(in.Remediation.TriggerIf.UnhealthyInRange)
	}
	if in.Remediation.TemplateRef.IsDefined() {
		out.RemediationTemplate = &corev1.ObjectReference{}
		if err := Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(&in.Remediation.TemplateRef, out.RemediationTemplate, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta2_MachineHealthCheckStatus_To_v1beta1_MachineHealthCheckStatus(in *clusterv1.MachineHealthCheckStatus, out *MachineHealthCheckStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineHealthCheckStatus_To_v1beta1_MachineHealthCheckStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1) from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
	}

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &MachineHealthCheckV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_MachineHealthCheckStatus_To_v1beta2_MachineHealthCheckStatus(in *MachineHealthCheckStatus, out *clusterv1.MachineHealthCheckStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineHealthCheckStatus_To_v1beta2_MachineHealthCheckStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move legacy conditions (v1beta1) to the deprecated field.
	if in.Conditions == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineHealthCheckDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	return nil
}

func Convert_v1beta2_MachineSetStatus_To_v1beta1_MachineSetStatus(in *clusterv1.MachineSetStatus, out *MachineSetStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineSetStatus_To_v1beta1_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.AvailableReplicas = 0
	out.ReadyReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage, and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.FullyLabeledReplicas = in.Deprecated.V1Beta1.FullyLabeledReplicas
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachineSetV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachineSetStatus_To_v1beta2_MachineSetStatus(in *MachineSetStatus, out *clusterv1.MachineSetStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineSetStatus_To_v1beta2_MachineSetStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.AvailableReplicas = nil
	out.ReadyReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move legacy conditions (v1beta1), failureReason, failureMessage, and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineSetDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineSetV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.FullyLabeledReplicas = in.FullyLabeledReplicas
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_MachineStatus_To_v1beta1_MachineStatus(in *clusterv1.MachineStatus, out *MachineStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineStatus_To_v1beta1_MachineStatus(in, out, s); err != nil {
		return err
	}

	// Convert v1beta2 Updating phase to v1beta1 Running as Updating did not exist in v1beta1.
	// We don't have to support a round-trip as only the core CAPI controller should write the Phase field.
	if out.Phase == "Updating" {
		out.Phase = "Running"
	}

	if !reflect.DeepEqual(in.LastUpdated, metav1.Time{}) {
		out.LastUpdated = ptr.To(in.LastUpdated)
	}
	if !reflect.DeepEqual(in.CertificatesExpiryDate, metav1.Time{}) {
		out.CertificatesExpiryDate = ptr.To(in.CertificatesExpiryDate)
	}
	if in.NodeRef.IsDefined() {
		out.NodeRef = &corev1.ObjectReference{
			Name:       in.NodeRef.Name,
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
		}
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Retrieve legacy conditions (v1beta1), failureReason and failureMessage from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
	}

	// Move initialization to old fields
	out.BootstrapReady = ptr.Deref(in.Initialization.BootstrapDataSecretCreated, false)
	out.InfrastructureReady = ptr.Deref(in.Initialization.InfrastructureProvisioned, false)

	// Move new conditions (v1beta2) to the v1beta2 field.
	if in.Conditions == nil {
		return nil
	}
	out.V1Beta2 = &MachineV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	return nil
}

func Convert_v1beta1_MachineStatus_To_v1beta2_MachineStatus(in *MachineStatus, out *clusterv1.MachineStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineStatus_To_v1beta2_MachineStatus(in, out, s); err != nil {
		return err
	}

	if in.LastUpdated != nil && !reflect.DeepEqual(in.LastUpdated, &metav1.Time{}) {
		out.LastUpdated = *in.LastUpdated
	}
	if in.CertificatesExpiryDate != nil && !reflect.DeepEqual(in.CertificatesExpiryDate, &metav1.Time{}) {
		out.CertificatesExpiryDate = *in.CertificatesExpiryDate
	}
	if in.NodeRef != nil && !reflect.DeepEqual(in.NodeRef, &corev1.ObjectReference{}) {
		out.NodeRef.Name = in.NodeRef.Name
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Retrieve new conditions (v1beta2) from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
	}

	// Move BootstrapReady and InfrastructureReady to Initialization is implemented in ConvertTo.

	// Move legacy conditions (v1beta1), failureReason and failureMessage to the deprecated field.
	if in.Conditions == nil && in.FailureReason == nil && in.FailureMessage == nil {
		return nil
	}

	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachineDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	return nil
}

func Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(in *clusterv1.Conditions, out *Conditions) {
	*out = make(Conditions, len(*in))
	for i := range *in {
		(*out)[i] = *(*Condition)(unsafe.Pointer(&(*in)[i])) //nolint:gosec
	}
}

func Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(in *Conditions, out *clusterv1.Conditions) {
	*out = make(clusterv1.Conditions, len(*in))
	for i := range *in {
		(*out)[i] = *(*clusterv1.Condition)(unsafe.Pointer(&(*in)[i])) //nolint:gosec
	}
}

func Convert_v1_Condition_To_v1beta1_Condition(_ *metav1.Condition, _ *Condition, _ apimachineryconversion.Scope) error {
	// NOTE: v1beta2 conditions should not be automatically converted into legacy (v1beta1) conditions.
	return nil
}

func Convert_v1beta1_Condition_To_v1_Condition(_ *Condition, _ *metav1.Condition, _ apimachineryconversion.Scope) error {
	// NOTE: legacy (v1beta1) conditions should not be automatically converted into v1beta2 conditions.
	return nil
}

func Convert_v1beta1_ClusterVariable_To_v1beta2_ClusterVariable(in *ClusterVariable, out *clusterv1.ClusterVariable, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 ClusterVariable does not have DefinitionFrom anymore. But it's fine to just lose this field,
	// because it was already not possible to set it anymore with v1beta1.
	return autoConvert_v1beta1_ClusterVariable_To_v1beta2_ClusterVariable(in, out, s)
}

func Convert_v1beta2_MachineSpec_To_v1beta1_MachineSpec(in *clusterv1.MachineSpec, out *MachineSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineSpec_To_v1beta1_MachineSpec(in, out, s); err != nil {
		return err
	}
	out.NodeDrainTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDrainTimeoutSeconds)
	out.NodeVolumeDetachTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeVolumeDetachTimeoutSeconds)
	out.NodeDeletionTimeout = clusterv1.ConvertFromSeconds(in.Deletion.NodeDeletionTimeoutSeconds)
	return nil
}

func Convert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in *clusterv1.MachinePoolStatus, out *MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachinePoolStatus_To_v1beta1_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta2 conditions should not be automatically be converted into legacy conditions (v1beta1).
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: replica counters with a new semantic should not be automatically be converted into old replica counters.
	out.ReadyReplicas = 0
	out.AvailableReplicas = 0

	// Retrieve legacy conditions (v1beta1), failureReason, failureMessage and replica counters from the deprecated field.
	if in.Deprecated != nil && in.Deprecated.V1Beta1 != nil {
		if in.Deprecated.V1Beta1.Conditions != nil {
			Convert_v1beta2_Deprecated_V1Beta1_Conditions_To_v1beta1_Conditions(&in.Deprecated.V1Beta1.Conditions, &out.Conditions)
		}
		out.FailureReason = in.Deprecated.V1Beta1.FailureReason
		out.FailureMessage = in.Deprecated.V1Beta1.FailureMessage
		out.ReadyReplicas = in.Deprecated.V1Beta1.ReadyReplicas
		out.AvailableReplicas = in.Deprecated.V1Beta1.AvailableReplicas
		out.UnavailableReplicas = in.Deprecated.V1Beta1.UnavailableReplicas
	}

	// Move initialization to old fields
	out.BootstrapReady = ptr.Deref(in.Initialization.BootstrapDataSecretCreated, false)
	out.InfrastructureReady = ptr.Deref(in.Initialization.InfrastructureProvisioned, false)

	// Move new conditions (v1beta2) and replica counters to the v1beta2 field.
	if in.Conditions == nil && in.ReadyReplicas == nil && in.AvailableReplicas == nil && in.UpToDateReplicas == nil {
		return nil
	}
	out.V1Beta2 = &MachinePoolV1Beta2Status{}
	out.V1Beta2.Conditions = in.Conditions
	out.V1Beta2.ReadyReplicas = in.ReadyReplicas
	out.V1Beta2.AvailableReplicas = in.AvailableReplicas
	out.V1Beta2.UpToDateReplicas = in.UpToDateReplicas
	return nil
}

func Convert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in *MachinePoolStatus, out *clusterv1.MachinePoolStatus, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachinePoolStatus_To_v1beta2_MachinePoolStatus(in, out, s); err != nil {
		return err
	}

	// Reset conditions from autogenerated conversions
	// NOTE: v1beta1 conditions should not be automatically be converted into v1beta2 conditions.
	out.Conditions = nil

	// Reset replica counters from autogenerated conversions
	// NOTE: old replica counters should not be automatically be converted into replica counters with a new semantic.
	out.ReadyReplicas = nil
	out.AvailableReplicas = nil

	// Retrieve new conditions (v1beta2) and replica counters from the v1beta2 field.
	if in.V1Beta2 != nil {
		out.Conditions = in.V1Beta2.Conditions
		out.ReadyReplicas = in.V1Beta2.ReadyReplicas
		out.AvailableReplicas = in.V1Beta2.AvailableReplicas
		out.UpToDateReplicas = in.V1Beta2.UpToDateReplicas
	}

	// Move BootstrapReady and InfrastructureReady to Initialization is implemented in ConvertTo.

	// Move legacy conditions (v1beta1), failureReason, failureMessage and replica counters to the deprecated field.
	if out.Deprecated == nil {
		out.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
	}
	if out.Deprecated.V1Beta1 == nil {
		out.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
	}
	if in.Conditions != nil {
		Convert_v1beta1_Conditions_To_v1beta2_Deprecated_V1Beta1_Conditions(&in.Conditions, &out.Deprecated.V1Beta1.Conditions)
	}
	out.Deprecated.V1Beta1.FailureReason = in.FailureReason
	out.Deprecated.V1Beta1.FailureMessage = in.FailureMessage
	out.Deprecated.V1Beta1.ReadyReplicas = in.ReadyReplicas
	out.Deprecated.V1Beta1.AvailableReplicas = in.AvailableReplicas
	out.Deprecated.V1Beta1.UnavailableReplicas = in.UnavailableReplicas
	return nil
}

func Convert_v1beta1_MachinePoolSpec_To_v1beta2_MachinePoolSpec(in *MachinePoolSpec, out *clusterv1.MachinePoolSpec, s apimachineryconversion.Scope) error {
	return autoConvert_v1beta1_MachinePoolSpec_To_v1beta2_MachinePoolSpec(in, out, s)
}

func Convert_v1beta1_ClusterClassStatusVariableDefinition_To_v1beta2_ClusterClassStatusVariableDefinition(in *ClusterClassStatusVariableDefinition, out *clusterv1.ClusterClassStatusVariableDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassStatusVariableDefinition_To_v1beta2_ClusterClassStatusVariableDefinition(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta1_ClusterClassVariableMetadata_To_v1beta2_ClusterClassVariableMetadata(&in.Metadata, &out.DeprecatedV1Beta1Metadata, s)
}

func Convert_v1beta2_ClusterClassStatusVariableDefinition_To_v1beta1_ClusterClassStatusVariableDefinition(in *clusterv1.ClusterClassStatusVariableDefinition, out *ClusterClassStatusVariableDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassStatusVariableDefinition_To_v1beta1_ClusterClassStatusVariableDefinition(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_ClusterClassVariableMetadata_To_v1beta1_ClusterClassVariableMetadata(&in.DeprecatedV1Beta1Metadata, &out.Metadata, s)
}

func Convert_v1beta1_ClusterClassVariable_To_v1beta2_ClusterClassVariable(in *ClusterClassVariable, out *clusterv1.ClusterClassVariable, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterClassVariable_To_v1beta2_ClusterClassVariable(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta1_ClusterClassVariableMetadata_To_v1beta2_ClusterClassVariableMetadata(&in.Metadata, &out.DeprecatedV1Beta1Metadata, s)
}

func Convert_v1beta2_ClusterClassVariable_To_v1beta1_ClusterClassVariable(in *clusterv1.ClusterClassVariable, out *ClusterClassVariable, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterClassVariable_To_v1beta1_ClusterClassVariable(in, out, s); err != nil {
		return err
	}
	return autoConvert_v1beta2_ClusterClassVariableMetadata_To_v1beta1_ClusterClassVariableMetadata(&in.DeprecatedV1Beta1Metadata, &out.Metadata, s)
}

func Convert_v1beta1_ExternalPatchDefinition_To_v1beta2_ExternalPatchDefinition(in *ExternalPatchDefinition, out *clusterv1.ExternalPatchDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ExternalPatchDefinition_To_v1beta2_ExternalPatchDefinition(in, out, s); err != nil {
		return err
	}

	out.GeneratePatchesExtension = ptr.Deref(in.GenerateExtension, "")
	out.ValidateTopologyExtension = ptr.Deref(in.ValidateExtension, "")
	return nil
}

func Convert_v1beta2_ExternalPatchDefinition_To_v1beta1_ExternalPatchDefinition(in *clusterv1.ExternalPatchDefinition, out *ExternalPatchDefinition, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ExternalPatchDefinition_To_v1beta1_ExternalPatchDefinition(in, out, s); err != nil {
		return err
	}

	if in.GeneratePatchesExtension != "" {
		out.GenerateExtension = ptr.To(in.GeneratePatchesExtension)
	}
	if in.ValidateTopologyExtension != "" {
		out.ValidateExtension = ptr.To(in.ValidateTopologyExtension)
	}
	return nil
}

func Convert_v1_ObjectReference_To_v1beta2_MachineHealthCheckRemediationTemplateReference(in *corev1.ObjectReference, out *clusterv1.MachineHealthCheckRemediationTemplateReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	out.Kind = in.Kind
	out.APIVersion = in.APIVersion
	return nil
}

func Convert_v1beta2_MachineHealthCheckRemediationTemplateReference_To_v1_ObjectReference(in *clusterv1.MachineHealthCheckRemediationTemplateReference, out *corev1.ObjectReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	out.Kind = in.Kind
	out.APIVersion = in.APIVersion
	return nil
}

func Convert_v1_ObjectReference_To_v1beta2_ContractVersionedObjectReference(_ *corev1.ObjectReference, _ *clusterv1.ContractVersionedObjectReference, _ apimachineryconversion.Scope) error {
	// This is implemented in ConvertTo/ConvertFrom as we have all necessary information available there.
	return nil
}

func Convert_v1beta2_ContractVersionedObjectReference_To_v1_ObjectReference(_ *clusterv1.ContractVersionedObjectReference, _ *corev1.ObjectReference, _ apimachineryconversion.Scope) error {
	// This is implemented in ConvertTo/ConvertFrom as we have all necessary information available there.
	return nil
}

func Convert_v1_ObjectReference_To_v1beta2_MachineNodeReference(in *corev1.ObjectReference, out *clusterv1.MachineNodeReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	return nil
}

func Convert_v1beta2_MachineNodeReference_To_v1_ObjectReference(in *clusterv1.MachineNodeReference, out *corev1.ObjectReference, _ apimachineryconversion.Scope) error {
	out.Name = in.Name
	out.APIVersion = corev1.SchemeGroupVersion.String()
	out.Kind = "Node"
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_ControlPlaneClassMachineInfrastructureTemplate(in *LocalObjectTemplate, out *clusterv1.ControlPlaneClassMachineInfrastructureTemplate, s apimachineryconversion.Scope) error {
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachineDeploymentClassBootstrapTemplate(in *LocalObjectTemplate, out *clusterv1.MachineDeploymentClassBootstrapTemplate, s apimachineryconversion.Scope) error {
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachineDeploymentClassInfrastructureTemplate(in *LocalObjectTemplate, out *clusterv1.MachineDeploymentClassInfrastructureTemplate, s apimachineryconversion.Scope) error {
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachinePoolClassBootstrapTemplate(in *LocalObjectTemplate, out *clusterv1.MachinePoolClassBootstrapTemplate, s apimachineryconversion.Scope) error {
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func Convert_v1beta1_LocalObjectTemplate_To_v1beta2_MachinePoolClassInfrastructureTemplate(in *LocalObjectTemplate, out *clusterv1.MachinePoolClassInfrastructureTemplate, s apimachineryconversion.Scope) error {
	convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in, &out.TemplateRef, s)
	return nil
}

func convert_v1beta1_LocalObjectTemplate_To_v1beta2_ClusterClassTemplateReference(in *LocalObjectTemplate, out *clusterv1.ClusterClassTemplateReference, _ apimachineryconversion.Scope) {
	if in == nil || in.Ref == nil {
		return
	}

	*out = clusterv1.ClusterClassTemplateReference{
		Kind:       in.Ref.Kind,
		Name:       in.Ref.Name,
		APIVersion: in.Ref.APIVersion,
	}
}

func Convert_v1beta2_ControlPlaneClassMachineInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.ControlPlaneClassMachineInfrastructureTemplate, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta2_MachineDeploymentClassBootstrapTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.MachineDeploymentClassBootstrapTemplate, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta2_MachineDeploymentClassInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.MachineDeploymentClassInfrastructureTemplate, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta2_MachinePoolClassBootstrapTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.MachinePoolClassBootstrapTemplate, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta2_MachinePoolClassInfrastructureTemplate_To_v1beta1_LocalObjectTemplate(in *clusterv1.MachinePoolClassInfrastructureTemplate, out *LocalObjectTemplate, s apimachineryconversion.Scope) error {
	Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(&in.TemplateRef, out, s)
	return nil
}

func Convert_v1beta2_ClusterClassTemplateReference_To_v1beta1_LocalObjectTemplate(in *clusterv1.ClusterClassTemplateReference, out *LocalObjectTemplate, _ apimachineryconversion.Scope) {
	if in == nil {
		return
	}

	out.Ref = &corev1.ObjectReference{
		Kind:       in.Kind,
		Name:       in.Name,
		APIVersion: in.APIVersion,
	}
}

func Convert_v1beta1_MachineSetSpec_To_v1beta2_MachineSetSpec(in *MachineSetSpec, out *clusterv1.MachineSetSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_MachineSetSpec_To_v1beta2_MachineSetSpec(in, out, s); err != nil {
		return err
	}

	out.Deletion.Order = clusterv1.MachineSetDeletionOrder(in.DeletePolicy)
	if in.MachineNamingStrategy != nil {
		out.MachineNaming = clusterv1.MachineNamingSpec{
			Template: in.MachineNamingStrategy.Template,
		}
	}
	return nil
}

func Convert_v1beta2_MachineSetSpec_To_v1beta1_MachineSetSpec(in *clusterv1.MachineSetSpec, out *MachineSetSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineSetSpec_To_v1beta1_MachineSetSpec(in, out, s); err != nil {
		return err
	}

	out.DeletePolicy = string(in.Deletion.Order)
	if in.MachineNaming.Template != "" {
		out.MachineNamingStrategy = &MachineNamingStrategy{
			Template: in.MachineNaming.Template,
		}
	}
	return nil
}

func Convert_v1beta1_ClusterSpec_To_v1beta2_ClusterSpec(in *ClusterSpec, out *clusterv1.ClusterSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterSpec_To_v1beta2_ClusterSpec(in, out, s); err != nil {
		return err
	}
	if in.ClusterNetwork != nil {
		if err := Convert_v1beta1_ClusterNetwork_To_v1beta2_ClusterNetwork(in.ClusterNetwork, &out.ClusterNetwork, s); err != nil {
			return err
		}
	}
	if in.Topology != nil {
		if err := Convert_v1beta1_Topology_To_v1beta2_Topology(in.Topology, &out.Topology, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta2_ClusterSpec_To_v1beta1_ClusterSpec(in *clusterv1.ClusterSpec, out *ClusterSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterSpec_To_v1beta1_ClusterSpec(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.ClusterNetwork, clusterv1.ClusterNetwork{}) {
		out.ClusterNetwork = &ClusterNetwork{}
		if err := Convert_v1beta2_ClusterNetwork_To_v1beta1_ClusterNetwork(&in.ClusterNetwork, out.ClusterNetwork, s); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(in.Topology, clusterv1.Topology{}) {
		out.Topology = &Topology{}
		if err := Convert_v1beta2_Topology_To_v1beta1_Topology(&in.Topology, out.Topology, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta1_ClusterNetwork_To_v1beta2_ClusterNetwork(in *ClusterNetwork, out *clusterv1.ClusterNetwork, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_ClusterNetwork_To_v1beta2_ClusterNetwork(in, out, s); err != nil {
		return err
	}
	if in.Services != nil {
		if err := autoConvert_v1beta1_NetworkRanges_To_v1beta2_NetworkRanges(in.Services, &out.Services, s); err != nil {
			return err
		}
	}
	if in.Pods != nil {
		if err := autoConvert_v1beta1_NetworkRanges_To_v1beta2_NetworkRanges(in.Pods, &out.Pods, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta2_ClusterNetwork_To_v1beta1_ClusterNetwork(in *clusterv1.ClusterNetwork, out *ClusterNetwork, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_ClusterNetwork_To_v1beta1_ClusterNetwork(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Services, clusterv1.NetworkRanges{}) {
		out.Services = &NetworkRanges{}
		if err := autoConvert_v1beta2_NetworkRanges_To_v1beta1_NetworkRanges(&in.Services, out.Services, s); err != nil {
			return err
		}
	}

	if !reflect.DeepEqual(in.Pods, clusterv1.NetworkRanges{}) {
		out.Pods = &NetworkRanges{}
		if err := autoConvert_v1beta2_NetworkRanges_To_v1beta1_NetworkRanges(&in.Pods, out.Pods, s); err != nil {
			return err
		}
	}

	return nil
}

func Convert_v1beta1_MachineDeploymentSpec_To_v1beta2_MachineDeploymentSpec(in *MachineDeploymentSpec, out *clusterv1.MachineDeploymentSpec, s apimachineryconversion.Scope) error {
	// NOTE: v1beta2 MachineDeploymentSpec does not have ProgressDeadlineSeconds anymore. But it's fine to just lose this field it was never used.
	if err := autoConvert_v1beta1_MachineDeploymentSpec_To_v1beta2_MachineDeploymentSpec(in, out, s); err != nil {
		return err
	}
	if in.Strategy != nil {
		out.Rollout.Strategy.Type = clusterv1.MachineDeploymentRolloutStrategyType(in.Strategy.Type)
		if in.Strategy.RollingUpdate != nil {
			out.Rollout.Strategy.RollingUpdate.MaxUnavailable = in.Strategy.RollingUpdate.MaxUnavailable
			out.Rollout.Strategy.RollingUpdate.MaxSurge = in.Strategy.RollingUpdate.MaxSurge
			if in.Strategy.RollingUpdate.DeletePolicy != nil {
				out.Deletion.Order = clusterv1.MachineSetDeletionOrder(*in.Strategy.RollingUpdate.DeletePolicy)
			}
		}
		if in.Strategy.Remediation != nil && in.Strategy.Remediation.MaxInFlight != nil {
			out.Remediation.MaxInFlight = in.Strategy.Remediation.MaxInFlight
		}
	}
	if in.RolloutAfter != nil && !reflect.DeepEqual(in.RolloutAfter, &metav1.Time{}) {
		out.Rollout.After = *in.RolloutAfter
	}
	if in.MachineNamingStrategy != nil {
		out.MachineNaming = clusterv1.MachineNamingSpec{
			Template: in.MachineNamingStrategy.Template,
		}
	}

	return nil
}

func Convert_v1beta2_MachineDeploymentSpec_To_v1beta1_MachineDeploymentSpec(in *clusterv1.MachineDeploymentSpec, out *MachineDeploymentSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_MachineDeploymentSpec_To_v1beta1_MachineDeploymentSpec(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.Rollout.Strategy, clusterv1.MachineDeploymentRolloutStrategy{}) {
		out.Strategy = &MachineDeploymentStrategy{}
		out.Strategy.Type = MachineDeploymentStrategyType(in.Rollout.Strategy.Type)
		if !reflect.DeepEqual(in.Rollout.Strategy.RollingUpdate, clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{}) {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
			out.Strategy.RollingUpdate.MaxUnavailable = in.Rollout.Strategy.RollingUpdate.MaxUnavailable
			out.Strategy.RollingUpdate.MaxSurge = in.Rollout.Strategy.RollingUpdate.MaxSurge
		}
	}
	if in.Deletion.Order != "" {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.RollingUpdate == nil {
			out.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
		}
		out.Strategy.RollingUpdate.DeletePolicy = ptr.To(string(in.Deletion.Order))
	}
	if in.Remediation.MaxInFlight != nil {
		if out.Strategy == nil {
			out.Strategy = &MachineDeploymentStrategy{}
		}
		if out.Strategy.Remediation == nil {
			out.Strategy.Remediation = &RemediationStrategy{}
		}
		out.Strategy.Remediation.MaxInFlight = in.Remediation.MaxInFlight
	}
	if !reflect.DeepEqual(in.Rollout.After, metav1.Time{}) {
		out.RolloutAfter = ptr.To(in.Rollout.After)
	}
	if in.MachineNaming.Template != "" {
		out.MachineNamingStrategy = &MachineNamingStrategy{
			Template: in.MachineNaming.Template,
		}
	}
	return nil
}

func Convert_v1beta1_Bootstrap_To_v1beta2_Bootstrap(in *Bootstrap, out *clusterv1.Bootstrap, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_Bootstrap_To_v1beta2_Bootstrap(in, out, s); err != nil {
		return err
	}
	if in.ConfigRef != nil {
		if err := Convert_v1_ObjectReference_To_v1beta2_ContractVersionedObjectReference(in.ConfigRef, &out.ConfigRef, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_Bootstrap_To_v1beta1_Bootstrap(in *clusterv1.Bootstrap, out *Bootstrap, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_Bootstrap_To_v1beta1_Bootstrap(in, out, s); err != nil {
		return err
	}
	if in.ConfigRef.IsDefined() {
		out.ConfigRef = &corev1.ObjectReference{}
		if err := Convert_v1beta2_ContractVersionedObjectReference_To_v1_ObjectReference(&in.ConfigRef, out.ConfigRef, s); err != nil {
			return err
		}
	}
	return nil
}

func convertMachineSpecToContractVersionedObjectReference(src *MachineSpec, dst *clusterv1.MachineSpec) error {
	infraRef, err := convertToContractVersionedObjectReference(&src.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.InfrastructureRef = infraRef

	if src.Bootstrap.ConfigRef != nil {
		bootstrapRef, err := convertToContractVersionedObjectReference(src.Bootstrap.ConfigRef)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
}

func convertMachineSpecToObjectReference(src *clusterv1.MachineSpec, dst *MachineSpec, namespace string) error {
	if src.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(src.InfrastructureRef, namespace)
		if err != nil {
			return err
		}
		dst.InfrastructureRef = *infraRef
	}

	if src.Bootstrap.ConfigRef.IsDefined() {
		bootstrapRef, err := convertToObjectReference(src.Bootstrap.ConfigRef, namespace)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
}

func convertToContractVersionedObjectReference(ref *corev1.ObjectReference) (clusterv1.ContractVersionedObjectReference, error) {
	var apiGroup string
	if ref.APIVersion != "" {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("failed to convert object: failed to parse apiVersion: %v", err)
		}
		apiGroup = gv.Group
	}
	return clusterv1.ContractVersionedObjectReference{
		APIGroup: apiGroup,
		Kind:     ref.Kind,
		Name:     ref.Name,
	}, nil
}

func convertToObjectReference(ref clusterv1.ContractVersionedObjectReference, namespace string) (*corev1.ObjectReference, error) {
	apiVersion, err := apiVersionGetter(schema.GroupKind{
		Group: ref.APIGroup,
		Kind:  ref.Kind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to convert object: %v", err)
	}
	return &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}, nil
}

func Convert_v1beta1_JSONSchemaProps_To_v1beta2_JSONSchemaProps(in *JSONSchemaProps, out *clusterv1.JSONSchemaProps, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta1_JSONSchemaProps_To_v1beta2_JSONSchemaProps(in, out, s); err != nil {
		return err
	}
	if in.XMetadata != nil {
		if err := Convert_v1beta1_VariableSchemaMetadata_To_v1beta2_VariableSchemaMetadata(in.XMetadata, &out.XMetadata, s); err != nil {
			return err
		}
	}
	return nil
}

func Convert_v1beta2_JSONSchemaProps_To_v1beta1_JSONSchemaProps(in *clusterv1.JSONSchemaProps, out *JSONSchemaProps, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_JSONSchemaProps_To_v1beta1_JSONSchemaProps(in, out, s); err != nil {
		return err
	}
	if !reflect.DeepEqual(in.XMetadata, clusterv1.VariableSchemaMetadata{}) {
		out.XMetadata = &VariableSchemaMetadata{}
		if err := Convert_v1beta2_VariableSchemaMetadata_To_v1beta1_VariableSchemaMetadata(&in.XMetadata, out.XMetadata, s); err != nil {
			return err
		}
	}
	return nil
}

func dropEmptyStringsCluster(dst *Cluster) {
	if dst.Spec.Topology != nil {
		if dst.Spec.Topology.Workers != nil {
			for i, md := range dst.Spec.Topology.Workers.MachineDeployments {
				dropEmptyString(&md.FailureDomain)
				dst.Spec.Topology.Workers.MachineDeployments[i] = md
			}
		}
	}
}

func dropEmptyStringsClusterClass(dst *ClusterClass) {
	if dst.Spec.InfrastructureNamingStrategy != nil {
		dropEmptyString(&dst.Spec.InfrastructureNamingStrategy.Template)
	}

	if dst.Spec.ControlPlane.NamingStrategy != nil {
		dropEmptyString(&dst.Spec.ControlPlane.NamingStrategy.Template)
	}

	for i, md := range dst.Spec.Workers.MachineDeployments {
		if md.NamingStrategy != nil {
			dropEmptyString(&md.NamingStrategy.Template)
		}
		dropEmptyString(&md.FailureDomain)
		dst.Spec.Workers.MachineDeployments[i] = md
	}

	for i, mp := range dst.Spec.Workers.MachinePools {
		if mp.NamingStrategy != nil {
			dropEmptyString(&mp.NamingStrategy.Template)
		}

		dst.Spec.Workers.MachinePools[i] = mp
	}

	for i, p := range dst.Spec.Patches {
		dropEmptyString(&p.EnabledIf)
		if p.External != nil {
			dropEmptyString(&p.External.GenerateExtension)
			dropEmptyString(&p.External.ValidateExtension)
			dropEmptyString(&p.External.DiscoverVariablesExtension)
		}

		for j, d := range p.Definitions {
			for k, jp := range d.JSONPatches {
				if jp.ValueFrom != nil {
					dropEmptyString(&jp.ValueFrom.Variable)
					dropEmptyString(&jp.ValueFrom.Template)
				}
				d.JSONPatches[k] = jp
			}
			p.Definitions[j] = d
		}

		dst.Spec.Patches[i] = p
	}
}

func dropEmptyStringsMachineSpec(spec *MachineSpec) {
	dropEmptyString(&spec.Version)
	dropEmptyString(&spec.ProviderID)
	dropEmptyString(&spec.FailureDomain)
}

func dropEmptyString(s **string) {
	if *s != nil && **s == "" {
		*s = nil
	}
}

func Convert_v1beta1_MachineDeletionStatus_To_v1beta2_MachineDeletionStatus(in *MachineDeletionStatus, out *clusterv1.MachineDeletionStatus, _ apimachineryconversion.Scope) error {
	if in.NodeDrainStartTime != nil && !reflect.DeepEqual(in.NodeDrainStartTime, &metav1.Time{}) {
		out.NodeDrainStartTime = *in.NodeDrainStartTime
	}
	if in.WaitForNodeVolumeDetachStartTime != nil && !reflect.DeepEqual(in.WaitForNodeVolumeDetachStartTime, &metav1.Time{}) {
		out.WaitForNodeVolumeDetachStartTime = *in.WaitForNodeVolumeDetachStartTime
	}
	return nil
}

func Convert_v1beta2_MachineDeletionStatus_To_v1beta1_MachineDeletionStatus(in *clusterv1.MachineDeletionStatus, out *MachineDeletionStatus, _ apimachineryconversion.Scope) error {
	if !reflect.DeepEqual(in.NodeDrainStartTime, metav1.Time{}) {
		out.NodeDrainStartTime = ptr.To(in.NodeDrainStartTime)
	}
	if !reflect.DeepEqual(in.WaitForNodeVolumeDetachStartTime, metav1.Time{}) {
		out.WaitForNodeVolumeDetachStartTime = ptr.To(in.WaitForNodeVolumeDetachStartTime)
	}
	return nil
}
