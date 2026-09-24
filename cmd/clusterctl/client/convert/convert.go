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

// Package convert provides a converter for CAPI core resources between API versions.
package convert

import (
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

// Converter handles the conversion of CAPI core resources between API versions.
type Converter struct {
	scheme         *runtime.Scheme
	targetAPIGroup string
	sourceGVs      map[schema.GroupVersion]bool
}

// NewConverter creates a new Converter instance.
func NewConverter(targetAPIGroup string, sourceGroupVersions []schema.GroupVersion) *Converter {
	sourceGVs := make(map[schema.GroupVersion]bool, len(sourceGroupVersions))
	for _, gv := range sourceGroupVersions {
		sourceGVs[gv] = true
	}

	return &Converter{
		scheme:         scheme.Scheme,
		targetAPIGroup: targetAPIGroup,
		sourceGVs:      sourceGVs,
	}
}

// Result contains the result of a conversion operation.
type Result struct {
	// Output is the converted YAML content.
	Output []byte

	// Converted is the number of resources that were converted.
	Converted int

	// PassedThrough is the number of resources that were passed through unchanged.
	PassedThrough int
}

// Convert processes a multi-document YAML stream and converts resources to the target version.
func (c *Converter) Convert(input []byte, toVersion string) (Result, error) {
	targetGV := schema.GroupVersion{
		Group:   c.targetAPIGroup,
		Version: toVersion,
	}

	docs, err := parseYAMLStream(input, c.scheme, c.sourceGVs)
	if err != nil {
		return Result{}, pkgerrors.Wrap(err, "failed to parse YAML stream")
	}

	var converted, passedThrough int
	for i := range docs {
		doc := &docs[i]
		if isGenericList(doc.gvk) {
			docConverted, docPassedThrough, err := c.convertGenericList(doc, targetGV)
			if err != nil {
				return Result{}, err
			}
			converted += docConverted
			passedThrough += docPassedThrough
			continue
		}

		if !doc.convertible {
			passedThrough++
			continue
		}

		convertedObj, err := convertResource(doc.object, targetGV, c.scheme)
		if err != nil {
			return Result{}, pkgerrors.Wrapf(err, "failed to convert resource %s at index %d", doc.gvk.String(), doc.index)
		}
		doc.object = convertedObj
		converted++
	}

	output, err := serializeYAMLStream(docs, c.scheme)
	if err != nil {
		return Result{}, pkgerrors.Wrap(err, "failed to serialize output")
	}

	return Result{
		Output:        output,
		Converted:     converted,
		PassedThrough: passedThrough,
	}, nil
}

func (c *Converter) convertGenericList(doc *document, targetGV schema.GroupVersion) (int, int, error) {
	obj, ok := doc.object.(*unstructured.Unstructured)
	if !ok {
		return 0, 0, pkgerrors.Errorf("failed to convert List at index %d: expected unstructured object, got %T", doc.index, doc.object)
	}
	list, err := obj.ToList()
	if err != nil {
		return 0, 0, pkgerrors.Wrapf(err, "failed to decode List at index %d", doc.index)
	}

	typedDecoder := serializer.NewCodecFactory(c.scheme).UniversalDeserializer()
	items := make([]interface{}, len(list.Items))
	var converted, passedThrough int
	for i := range list.Items {
		gvk := list.Items[i].GroupVersionKind()
		if !c.sourceGVs[gvk.GroupVersion()] {
			items[i] = list.Items[i].Object
			passedThrough++
			continue
		}

		rawItem, err := list.Items[i].MarshalJSON()
		if err != nil {
			return 0, 0, pkgerrors.Wrapf(err, "failed to marshal List item at index %d", i)
		}
		item := runtime.Object(&list.Items[i])
		if typedItem, _, decodeErr := typedDecoder.Decode(rawItem, &gvk, nil); decodeErr == nil {
			item = typedItem
		}
		convertedItem, err := convertResource(item, targetGV, c.scheme)
		if err != nil {
			return 0, 0, pkgerrors.Wrapf(err, "failed to convert List item %s at index %d", gvk.String(), i)
		}
		items[i], err = runtime.DefaultUnstructuredConverter.ToUnstructured(convertedItem)
		if err != nil {
			return 0, 0, pkgerrors.Wrapf(err, "failed to serialize List item at index %d", i)
		}
		converted++
	}

	if converted == 0 {
		return 0, 1, nil
	}
	obj.Object["items"] = items
	return converted, passedThrough, nil
}

func isGenericList(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "" && gvk.Version == "v1" && gvk.Kind == "List"
}
