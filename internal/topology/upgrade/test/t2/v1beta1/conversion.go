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

package v1beta1

import (
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	testv1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *TestResourceTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*testv1.TestResourceTemplate)
	if err := Convert_v1beta1_TestResourceTemplate_To_v1beta2_TestResourceTemplate(src, dst, nil); err != nil {
		return err
	}

	if dst.Annotations == nil {
		dst.Annotations = map[string]string{}
	}
	dst.Annotations["conversionTo"] = ""

	// Manually restore data.
	restored := &testv1.TestResourceTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Template.Spec.BoolToPtrBool, ok, restored.Spec.Template.Spec.BoolToPtrBool, &dst.Spec.Template.Spec.BoolToPtrBool)
	Convert_int32_To_Pointer_int32(src.Spec.Template.Spec.Int32ToPtrInt32, ok, restored.Spec.Template.Spec.Int32ToPtrInt32, &dst.Spec.Template.Spec.Int32ToPtrInt32)
	return nil
}

func Convert_int32_To_Pointer_int32(in int32, hasRestored bool, restoredIn *int32, out **int32) {
	// If the value is 0, convert to *0 only if the value was *0 before (we know it was intentionally set to 0).
	// In all the other cases we do not know if the value was intentionally set to 0, so convert to nil.
	if in == 0 {
		if hasRestored && restoredIn != nil && *restoredIn == 0 {
			*out = ptr.To[int32](0)
			return
		}
		*out = nil
		return
	}

	// Otherwise, if the value is not 0, convert to *value.
	*out = ptr.To(in)
}

func (dst *TestResourceTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*testv1.TestResourceTemplate)

	if err := Convert_v1beta2_TestResourceTemplate_To_v1beta1_TestResourceTemplate(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.Template.Spec.PtrStringToString != nil && *dst.Spec.Template.Spec.PtrStringToString == "" {
		dst.Spec.Template.Spec.PtrStringToString = nil
	}
	return utilconversion.MarshalData(src, dst)
}

func (src *TestResource) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*testv1.TestResource)
	if err := Convert_v1beta1_TestResource_To_v1beta2_TestResource(src, dst, nil); err != nil {
		return err
	}

	if dst.Annotations == nil {
		dst.Annotations = map[string]string{}
	}
	dst.Annotations["conversionTo"] = ""

	// Manually restore data.
	restored := &testv1.TestResource{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.BoolToPtrBool, ok, restored.Spec.BoolToPtrBool, &dst.Spec.BoolToPtrBool)
	Convert_int32_To_Pointer_int32(src.Spec.Int32ToPtrInt32, ok, restored.Spec.Int32ToPtrInt32, &dst.Spec.Int32ToPtrInt32)
	return nil
}

func (dst *TestResource) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*testv1.TestResource)

	if err := Convert_v1beta2_TestResource_To_v1beta1_TestResource(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.PtrStringToString != nil && *dst.Spec.PtrStringToString == "" {
		dst.Spec.PtrStringToString = nil
	}

	return utilconversion.MarshalData(src, dst)
}
