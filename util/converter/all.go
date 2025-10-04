package converter
//
//import (
//	"fmt"
//	"reflect"
//
//	"k8s.io/utils/ptr"
//	"sigs.k8s.io/controller-runtime/pkg/conversion"
//
//	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
//)
//
//func (src *ClusterClass) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.ClusterClass)
//
//	if err := Convert_v1beta1_ClusterClass_To_v1beta2_ClusterClass(src, dst, nil); err != nil {
//		return err
//	}
//
//	restored := &clusterv1.ClusterClass{}
//	ok, err := utilconversion.UnmarshalData(src, restored)
//	if err != nil {
//		return err
//	}
//
//	// Recover intent for bool values converted to *bool.
//	for i, patch := range dst.Spec.Patches {
//		for j, definition := range patch.Definitions {
//			var srcDefinition = &PatchDefinition{}
//			for _, p := range src.Spec.Patches {
//				if p.Name == patch.Name {
//					if len(p.Definitions) == len(patch.Definitions) {
//						srcDefinition = &p.Definitions[j]
//					}
//					break
//				}
//			}
//			if srcDefinition == nil {
//				return fmt.Errorf("definition %d for patch %s not found in source data", j, patch.Name)
//			}
//			var restoredPatchMatchControlPlane, restoredPatchMatchInfrastructureCluster *bool
//			for _, p := range restored.Spec.Patches {
//				if p.Name == patch.Name {
//					if len(p.Definitions) == len(patch.Definitions) {
//						restoredPatchMatchInfrastructureCluster = p.Definitions[j].Selector.MatchResources.InfrastructureCluster
//						restoredPatchMatchControlPlane = p.Definitions[j].Selector.MatchResources.ControlPlane
//					}
//					break
//				}
//			}
//			clusterv1.Convert_bool_To_Pointer_bool(srcDefinition.Selector.MatchResources.InfrastructureCluster, ok, restoredPatchMatchInfrastructureCluster, &definition.Selector.MatchResources.InfrastructureCluster)
//			clusterv1.Convert_bool_To_Pointer_bool(srcDefinition.Selector.MatchResources.ControlPlane, ok, restoredPatchMatchControlPlane, &definition.Selector.MatchResources.ControlPlane)
//			dst.Spec.Patches[i].Definitions[j] = definition
//		}
//	}
//
//	for i, variable := range dst.Spec.Variables {
//		var srcVariable *ClusterClassVariable
//		for _, v := range src.Spec.Variables {
//			if v.Name == variable.Name {
//				srcVariable = &v
//				break
//			}
//		}
//		if srcVariable == nil {
//			return fmt.Errorf("variable %q not found in source data", variable.Name)
//		}
//		var restoredVariableOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		for _, v := range restored.Spec.Variables {
//			if v.Name == variable.Name {
//				restoredVariableOpenAPIV3Schema = &v.Schema.OpenAPIV3Schema
//				break
//			}
//		}
//		if err := restoreBoolIntentJSONSchemaProps(&srcVariable.Schema.OpenAPIV3Schema, &variable.Schema.OpenAPIV3Schema, ok, restoredVariableOpenAPIV3Schema); err != nil {
//			return err
//		}
//		dst.Spec.Variables[i] = variable
//	}
//
//	for i, variable := range dst.Status.Variables {
//		var srcVariable *ClusterClassStatusVariable
//		for _, v := range src.Status.Variables {
//			if v.Name == variable.Name {
//				srcVariable = &v
//				break
//			}
//		}
//		if srcVariable == nil {
//			return fmt.Errorf("variable %q not found in source data", variable.Name)
//		}
//		var restoredVariable *clusterv1.ClusterClassStatusVariable
//		var restoredVariableDefinitionsConflict *bool
//		for _, v := range restored.Status.Variables {
//			if v.Name == variable.Name {
//				restoredVariable = &v
//				restoredVariableDefinitionsConflict = v.DefinitionsConflict
//				break
//			}
//		}
//		clusterv1.Convert_bool_To_Pointer_bool(srcVariable.DefinitionsConflict, ok, restoredVariableDefinitionsConflict, &variable.DefinitionsConflict)
//
//		for j, definition := range variable.Definitions {
//			var srcDefinition *ClusterClassStatusVariableDefinition
//			for _, d := range srcVariable.Definitions {
//				if d.From == definition.From {
//					srcDefinition = &d
//				}
//			}
//			if srcDefinition == nil {
//				return fmt.Errorf("definition %d for variable %s not found in source data", j, variable.Name)
//			}
//			var restoredVariableOpenAPIV3Schema *clusterv1.JSONSchemaProps
//			if restoredVariable != nil {
//				for _, d := range restoredVariable.Definitions {
//					if d.From == definition.From {
//						restoredVariableOpenAPIV3Schema = &d.Schema.OpenAPIV3Schema
//					}
//				}
//			}
//			if err := restoreBoolIntentJSONSchemaProps(&srcDefinition.Schema.OpenAPIV3Schema, &definition.Schema.OpenAPIV3Schema, ok, restoredVariableOpenAPIV3Schema); err != nil {
//				return err
//			}
//			variable.Definitions[j] = definition
//		}
//		dst.Status.Variables[i] = variable
//	}
//
//	dst.Spec.KubernetesVersions = restored.Spec.KubernetesVersions
//
//	dst.Spec.Upgrade.External.GenerateUpgradePlanExtension = restored.Spec.Upgrade.External.GenerateUpgradePlanExtension
//
//	return nil
//}
//
//func restoreBoolIntentJSONSchemaProps(src *JSONSchemaProps, dst *clusterv1.JSONSchemaProps, hasRestored bool, restored *clusterv1.JSONSchemaProps) error {
//	var restoredUniqueItems, restoreExclusiveMaximum, restoredExclusiveMinimum, restoreXPreserveUnknownFields, restoredXIntOrString *bool
//	if restored != nil {
//		restoredUniqueItems = restored.UniqueItems
//		restoreExclusiveMaximum = restored.ExclusiveMaximum
//		restoredExclusiveMinimum = restored.ExclusiveMinimum
//		restoreXPreserveUnknownFields = restored.XPreserveUnknownFields
//		restoredXIntOrString = restored.XIntOrString
//	}
//	clusterv1.Convert_bool_To_Pointer_bool(src.UniqueItems, hasRestored, restoredUniqueItems, &dst.UniqueItems)
//	clusterv1.Convert_bool_To_Pointer_bool(src.ExclusiveMaximum, hasRestored, restoreExclusiveMaximum, &dst.ExclusiveMaximum)
//	clusterv1.Convert_bool_To_Pointer_bool(src.ExclusiveMinimum, hasRestored, restoredExclusiveMinimum, &dst.ExclusiveMinimum)
//	clusterv1.Convert_bool_To_Pointer_bool(src.XPreserveUnknownFields, hasRestored, restoreXPreserveUnknownFields, &dst.XPreserveUnknownFields)
//	clusterv1.Convert_bool_To_Pointer_bool(src.XIntOrString, hasRestored, restoredXIntOrString, &dst.XIntOrString)
//
//	for name, property := range dst.Properties {
//		srcProperty, ok := src.Properties[name]
//		if !ok {
//			return fmt.Errorf("property %s not found in source data", name)
//		}
//		var restoredPropertyOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil {
//			if v, ok := restored.Properties[name]; ok {
//				restoredPropertyOpenAPIV3Schema = &v
//			}
//		}
//		if err := restoreBoolIntentJSONSchemaProps(&srcProperty, &property, hasRestored, restoredPropertyOpenAPIV3Schema); err != nil {
//			return err
//		}
//		dst.Properties[name] = property
//	}
//	if src.AdditionalProperties != nil {
//		var restoredAdditionalPropertiesOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil {
//			restoredAdditionalPropertiesOpenAPIV3Schema = restored.AdditionalProperties
//		}
//		if err := restoreBoolIntentJSONSchemaProps(src.AdditionalProperties, dst.AdditionalProperties, hasRestored, restoredAdditionalPropertiesOpenAPIV3Schema); err != nil {
//			return err
//		}
//	}
//	if src.Items != nil {
//		var restoreItemsOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil {
//			restoreItemsOpenAPIV3Schema = restored.Items
//		}
//		if err := restoreBoolIntentJSONSchemaProps(src.Items, dst.Items, hasRestored, restoreItemsOpenAPIV3Schema); err != nil {
//			return err
//		}
//	}
//	for i, value := range dst.AllOf {
//		srcValue := src.AllOf[i]
//		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil && len(src.AllOf) == len(dst.AllOf) {
//			restoredValueOpenAPIV3Schema = &restored.AllOf[i]
//		}
//		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
//			return err
//		}
//		dst.AllOf[i] = value
//	}
//	for i, value := range dst.OneOf {
//		srcValue := src.OneOf[i]
//		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil && len(src.OneOf) == len(dst.OneOf) {
//			restoredValueOpenAPIV3Schema = &restored.OneOf[i]
//		}
//		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
//			return err
//		}
//		dst.OneOf[i] = value
//	}
//	for i, value := range dst.AnyOf {
//		srcValue := src.AnyOf[i]
//		var restoredValueOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil && len(src.AnyOf) == len(dst.AnyOf) {
//			restoredValueOpenAPIV3Schema = &restored.AnyOf[i]
//		}
//		if err := restoreBoolIntentJSONSchemaProps(&srcValue, &value, hasRestored, restoredValueOpenAPIV3Schema); err != nil {
//			return err
//		}
//		dst.AnyOf[i] = value
//	}
//	if src.Not != nil {
//		var restoredNotOpenAPIV3Schema *clusterv1.JSONSchemaProps
//		if restored != nil {
//			restoredNotOpenAPIV3Schema = restored.Not
//		}
//		if err := restoreBoolIntentJSONSchemaProps(src.Not, dst.Not, hasRestored, restoredNotOpenAPIV3Schema); err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//func (dst *ClusterClass) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.ClusterClass)
//	if err := Convert_v1beta2_ClusterClass_To_v1beta1_ClusterClass(src, dst, nil); err != nil {
//		return err
//	}
//
//	if dst.Spec.ControlPlane.MachineHealthCheck != nil && dst.Spec.ControlPlane.MachineHealthCheck.RemediationTemplate != nil {
//		dst.Spec.ControlPlane.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
//	}
//	for _, md := range dst.Spec.Workers.MachineDeployments {
//		if md.MachineHealthCheck != nil && md.MachineHealthCheck.RemediationTemplate != nil {
//			md.MachineHealthCheck.RemediationTemplate.Namespace = dst.Namespace
//		}
//	}
//	dropEmptyStringsClusterClass(dst)
//
//	return utilconversion.MarshalData(src, dst)
//}
//
//func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.Machine)
//
//	if err := Convert_v1beta1_Machine_To_v1beta2_Machine(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec, &dst.Spec); err != nil {
//		return err
//	}
//
//	restored := &clusterv1.Machine{}
//	ok, err := utilconversion.UnmarshalData(src, restored)
//	if err != nil {
//		return err
//	}
//
//	// Recover intent for bool values converted to *bool.
//	initialization := clusterv1.MachineInitializationStatus{}
//	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
//	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
//	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
//	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
//	if !reflect.DeepEqual(initialization, clusterv1.MachineInitializationStatus{}) {
//		dst.Status.Initialization = initialization
//	}
//
//	// Recover other values.
//	if ok {
//		dst.Spec.MinReadySeconds = restored.Spec.MinReadySeconds
//	}
//
//	return nil
//}
//
//func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.Machine)
//
//	if err := Convert_v1beta2_Machine_To_v1beta1_Machine(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToObjectReference(&src.Spec, &dst.Spec, src.Namespace); err != nil {
//		return err
//	}
//
//	dropEmptyStringsMachineSpec(&dst.Spec)
//
//	return utilconversion.MarshalData(src, dst)
//}
//
//func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.MachineSet)
//
//	if err := Convert_v1beta1_MachineSet_To_v1beta2_MachineSet(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
//		return err
//	}
//
//	if src.Spec.MinReadySeconds == 0 {
//		dst.Spec.Template.Spec.MinReadySeconds = nil
//	} else {
//		dst.Spec.Template.Spec.MinReadySeconds = &src.Spec.MinReadySeconds
//	}
//
//	return nil
//}
//
//func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.MachineSet)
//
//	if err := Convert_v1beta2_MachineSet_To_v1beta1_MachineSet(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
//		return err
//	}
//
//	dst.Spec.MinReadySeconds = ptr.Deref(src.Spec.Template.Spec.MinReadySeconds, 0)
//
//	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)
//	return nil
//}
//
//func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.MachineDeployment)
//
//	if err := Convert_v1beta1_MachineDeployment_To_v1beta2_MachineDeployment(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
//		return err
//	}
//
//	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds
//
//	restored := &clusterv1.MachineDeployment{}
//	ok, err := utilconversion.UnmarshalData(src, restored)
//	if err != nil {
//		return err
//	}
//
//	// Recover intent for bool values converted to *bool.
//	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)
//
//	return nil
//}
//
//func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.MachineDeployment)
//
//	if err := Convert_v1beta2_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
//		return err
//	}
//
//	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds
//
//	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)
//
//	return utilconversion.MarshalData(src, dst)
//}
//
//func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.MachineHealthCheck)
//
//	if err := Convert_v1beta1_MachineHealthCheck_To_v1beta2_MachineHealthCheck(src, dst, nil); err != nil {
//		return err
//	}
//
//	// Manually restore data.
//	restored := &clusterv1.MachineHealthCheck{}
//	ok, err := utilconversion.UnmarshalData(src, restored)
//	if err != nil {
//		return err
//	}
//
//	clusterv1.Convert_int32_To_Pointer_int32(src.Status.ExpectedMachines, ok, restored.Status.ExpectedMachines, &dst.Status.ExpectedMachines)
//	clusterv1.Convert_int32_To_Pointer_int32(src.Status.CurrentHealthy, ok, restored.Status.CurrentHealthy, &dst.Status.CurrentHealthy)
//	clusterv1.Convert_int32_To_Pointer_int32(src.Status.RemediationsAllowed, ok, restored.Status.RemediationsAllowed, &dst.Status.RemediationsAllowed)
//
//	return nil
//}
//
//func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.MachineHealthCheck)
//	if err := Convert_v1beta2_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil); err != nil {
//		return err
//	}
//
//	if dst.Spec.RemediationTemplate != nil {
//		dst.Spec.RemediationTemplate.Namespace = src.Namespace
//	}
//
//	return utilconversion.MarshalData(src, dst)
//}
//
//func (src *MachinePool) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.MachinePool)
//
//	if err := Convert_v1beta1_MachinePool_To_v1beta2_MachinePool(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
//		return err
//	}
//
//	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds
//
//	restored := &clusterv1.MachinePool{}
//	ok, err := utilconversion.UnmarshalData(src, restored)
//	if err != nil {
//		return err
//	}
//
//	// Recover intent for bool values converted to *bool.
//	initialization := clusterv1.MachinePoolInitializationStatus{}
//	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
//	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
//	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
//	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
//	if !reflect.DeepEqual(initialization, clusterv1.MachinePoolInitializationStatus{}) {
//		dst.Status.Initialization = initialization
//	}
//
//	return nil
//}
//
//func (dst *MachinePool) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.MachinePool)
//
//	if err := Convert_v1beta2_MachinePool_To_v1beta1_MachinePool(src, dst, nil); err != nil {
//		return err
//	}
//
//	if err := convertMachineSpecToObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
//		return err
//	}
//
//	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds
//
//	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)
//
//	return utilconversion.MarshalData(src, dst)
//}
//
//func (src *MachineDrainRule) ConvertTo(dstRaw conversion.Hub) error {
//	dst := dstRaw.(*clusterv1.MachineDrainRule)
//	return Convert_v1beta1_MachineDrainRule_To_v1beta2_MachineDrainRule(src, dst, nil)
//}
//
//func (dst *MachineDrainRule) ConvertFrom(srcRaw conversion.Hub) error {
//	src := srcRaw.(*clusterv1.MachineDrainRule)
//	return Convert_v1beta2_MachineDrainRule_To_v1beta1_MachineDrainRule(src, dst, nil)
//}
