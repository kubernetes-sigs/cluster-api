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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

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

	// TODO: implement unit test on when nil is set
	if !ok {
		dst.Spec.Template.Spec.BoolToPtrBool = nil
		dst.Spec.Template.Spec.Int32ToPtrInt32 = nil
		return nil
	}

	if dst.Spec.Template.Spec.BoolToPtrBool != nil && !*dst.Spec.Template.Spec.BoolToPtrBool && restored.Spec.Template.Spec.BoolToPtrBool == nil {
		dst.Spec.Template.Spec.BoolToPtrBool = nil
	}
	if dst.Spec.Template.Spec.Int32ToPtrInt32 != nil && *dst.Spec.Template.Spec.Int32ToPtrInt32 == 0 && restored.Spec.Template.Spec.Int32ToPtrInt32 == nil {
		dst.Spec.Template.Spec.Int32ToPtrInt32 = nil
	}

	return nil
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

	if !ok {
		dst.Spec.BoolToPtrBool = nil
		dst.Spec.Int32ToPtrInt32 = nil
		return nil
	}

	if dst.Spec.BoolToPtrBool != nil && !*dst.Spec.BoolToPtrBool && restored.Spec.BoolToPtrBool == nil {
		dst.Spec.BoolToPtrBool = nil
	}
	if !ok || dst.Spec.Int32ToPtrInt32 != nil && *dst.Spec.Int32ToPtrInt32 == 0 && restored.Spec.Int32ToPtrInt32 == nil {
		dst.Spec.Int32ToPtrInt32 = nil
	}

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
