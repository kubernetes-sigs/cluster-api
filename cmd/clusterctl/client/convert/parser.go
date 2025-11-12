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

package convert

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

// document represents a single YAML document with associated metadata.
type document struct {
	object runtime.Object
	raw    []byte
	gvk    schema.GroupVersionKind
	typ    resourceType
	index  int
}

// resourceType classifies the type of Kubernetes resource.
type resourceType int

const (
	// resourceTypeConvertible identifies resources that can be converted (match source GVKs).
	resourceTypeConvertible resourceType = iota
	// resourceTypeKnown identifies resources in known groups but not convertible.
	resourceTypeKnown
	// resourceTypePassThrough identifies resources that should pass through unchanged.
	resourceTypePassThrough
)

// gvkMatcher provides GVK matching logic for resource classification.
type gvkMatcher struct {
	// sourceGroupVersions are GroupVersions where all kinds should be converted.
	sourceGroupVersions map[schema.GroupVersion]bool
	// knownGroups are API groups that are known to the scheme.
	knownGroups map[string]bool
}

// parseYAMLStream parses a multi-document YAML stream from a byte slice into individual documents.
func parseYAMLStream(input []byte, scheme *runtime.Scheme, matcher *gvkMatcher) ([]document, error) {
	reader := bytes.NewReader(input)
	yamlReader := yamlutil.NewYAMLReader(bufio.NewReader(reader))

	codecFactory := serializer.NewCodecFactory(scheme)
	unstructuredDecoder := yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	typedDecoder := codecFactory.UniversalDeserializer()

	var documents []document
	index := 0

	for {
		raw, err := yamlReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to read YAML document")
		}

		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			continue
		}

		doc, err := parseDocument(trimmed, raw, index, unstructuredDecoder, typedDecoder, matcher)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse document at index %d", index)
		}
		documents = append(documents, doc)
		index++
	}

	return documents, nil
}

// serializeYAMLStream writes documents back out as a multi-document YAML stream.
func serializeYAMLStream(docs []document, scheme *runtime.Scheme) ([]byte, error) {
	if len(docs) == 0 {
		return []byte{}, nil
	}

	yamlSerializer := jsonserializer.NewSerializerWithOptions(
		jsonserializer.DefaultMetaFactory,
		scheme,
		scheme,
		jsonserializer.SerializerOptions{Yaml: true, Pretty: false, Strict: false},
	)

	buf := &bytes.Buffer{}

	for i, doc := range docs {
		// Add document separator before each document except the first.
		if i > 0 {
			if _, err := buf.WriteString("---\n"); err != nil {
				return nil, errors.Wrap(err, "failed to write document separator")
			}
		}

		if doc.object != nil {
			docBuf := &bytes.Buffer{}
			if err := yamlSerializer.Encode(doc.object, docBuf); err != nil {
				return nil, errors.Wrapf(err, "failed to encode document at index %d", doc.index)
			}
			if _, err := buf.Write(ensureTrailingNewline(docBuf.Bytes())); err != nil {
				return nil, errors.Wrapf(err, "failed to write document at index %d", doc.index)
			}
			continue
		}

		if _, err := buf.Write(ensureTrailingNewline(doc.raw)); err != nil {
			return nil, errors.Wrapf(err, "failed to write raw document at index %d", doc.index)
		}
	}

	return buf.Bytes(), nil
}

// parseDocument parses a single YAML document and classifies it by resource type.
func parseDocument(trimmed []byte, original []byte, index int, unstructuredDecoder runtime.Decoder, typedDecoder runtime.Decoder, matcher *gvkMatcher) (document, error) {
	obj := &unstructured.Unstructured{}
	_, gvk, err := unstructuredDecoder.Decode(trimmed, nil, obj)
	if err != nil {
		return document{}, errors.Wrap(err, "unable to decode document: invalid YAML structure")
	}
	if gvk == nil || gvk.Empty() || gvk.Kind == "" || (gvk.Group == "" && gvk.Version == "") {
		return document{}, errors.New("unable to decode document: missing or empty apiVersion/kind")
	}

	resourceType := classifyResource(*gvk, matcher)

	var runtimeObj runtime.Object
	if resourceType == resourceTypeConvertible || resourceType == resourceTypeKnown {
		if typedObj, _, err := typedDecoder.Decode(trimmed, gvk, nil); err == nil {
			runtimeObj = typedObj
		} else {
			runtimeObj = obj
		}
	} else {
		runtimeObj = obj
	}

	return document{
		object: runtimeObj,
		raw:    ensureTrailingNewline(original),
		gvk:    *gvk,
		typ:    resourceType,
		index:  index,
	}, nil
}

// classifyResource determines the resource type based on its GroupVersionKind and the provided matcher.
func classifyResource(gvk schema.GroupVersionKind, matcher *gvkMatcher) resourceType {
	// Check if this GroupVersion should be converted
	gv := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
	if matcher.sourceGroupVersions[gv] {
		return resourceTypeConvertible
	}

	// Check if this is in a known group but not a source GroupVersion.
	if matcher.knownGroups[gvk.Group] {
		return resourceTypeKnown
	}

	// Everything else passes through
	return resourceTypePassThrough
}

// newGVKMatcher creates a new GVK matcher from source GroupVersions and known groups.
func newGVKMatcher(sourceGVs []schema.GroupVersion, knownGroups []string) *gvkMatcher {
	matcher := &gvkMatcher{
		sourceGroupVersions: make(map[schema.GroupVersion]bool),
		knownGroups:         make(map[string]bool),
	}

	for _, gv := range sourceGVs {
		matcher.sourceGroupVersions[gv] = true
	}

	for _, group := range knownGroups {
		matcher.knownGroups[group] = true
	}

	return matcher
}

// ensureTrailingNewline ensures that the content ends with a newline character.
func ensureTrailingNewline(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	if content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	return content
}
