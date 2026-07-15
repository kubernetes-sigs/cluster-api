/*
Copyright 2026 The Kubernetes Authors.

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

package conversion

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// ClusterClass is a HubSpokeConverter for the ClusterClass API type.
var ClusterClass = conversion.NewHubSpokeConverter(&clusterv1.ClusterClass{},
	conversion.NewSpokeConverter(&clusterv1beta1.ClusterClass{}, ConvertClusterClassHubToV1Beta1, ConvertClusterClassV1Beta1ToHub),
)

// ConvertClusterClassV1Beta1ToHub converts a v1beta1 ClusterClass to a hub ClusterClass.
func ConvertClusterClassV1Beta1ToHub(_ context.Context, src *clusterv1beta1.ClusterClass, dst *clusterv1.ClusterClass) error {
	if err := clusterv1beta1.Convert_v1beta1_ClusterClass_To_v1beta2_ClusterClass(src, dst, nil); err != nil {
		return err
	}

	restored := &clusterv1.ClusterClass{}
	ok, err := conversionutil.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	if ok {
		// Restore fields of the health checks that do not exist in v1beta1.
		dst.Spec.ControlPlane.HealthCheck.Checks.NodeDeleting = restored.Spec.ControlPlane.HealthCheck.Checks.NodeDeleting
		for i, md := range dst.Spec.Workers.MachineDeployments {
			for _, restoredMD := range restored.Spec.Workers.MachineDeployments {
				if restoredMD.Class == md.Class {
					dst.Spec.Workers.MachineDeployments[i].HealthCheck.Checks.NodeDeleting = restoredMD.HealthCheck.Checks.NodeDeleting
					break
				}
			}
		}
	}

	// Recover intent for bool values converted to *bool.
	for i, patch := range dst.Spec.Patches {
		for j, definition := range patch.Definitions {
			var srcDefinition = &clusterv1beta1.PatchDefinition{}
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
		var srcVariable *clusterv1beta1.ClusterClassVariable
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
		var srcVariable *clusterv1beta1.ClusterClassStatusVariable
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
			var srcDefinition *clusterv1beta1.ClusterClassStatusVariableDefinition
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

	return nil
}

func restoreBoolIntentJSONSchemaProps(src *clusterv1beta1.JSONSchemaProps, dst *clusterv1.JSONSchemaProps, hasRestored bool, restored *clusterv1.JSONSchemaProps) error {
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

// ConvertClusterClassHubToV1Beta1 converts a hub ClusterClass to a v1beta1 ClusterClass.
func ConvertClusterClassHubToV1Beta1(_ context.Context, src *clusterv1.ClusterClass, dst *clusterv1beta1.ClusterClass) error {
	if err := clusterv1beta1.Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(src, dst, nil); err != nil {
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

	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
}

func dropEmptyStringsClusterClass(dst *clusterv1beta1.ClusterClass) {
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
