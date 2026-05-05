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
	"k8s.io/apimachinery/pkg/runtime/serializer"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	yamlserializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

// document represents a single parsed YAML document.
type document struct {
	object      runtime.Object
	gvk         schema.GroupVersionKind
	convertible bool
	index       int
}

// parseYAMLStream parses a multi-document YAML stream into individual documents.
func parseYAMLStream(input []byte, scheme *runtime.Scheme, sourceGVs map[schema.GroupVersion]bool) ([]document, error) {
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

		doc, err := parseDocument(trimmed, index, unstructuredDecoder, typedDecoder, sourceGVs)
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
		if i > 0 {
			if _, err := buf.WriteString("---\n"); err != nil {
				return nil, errors.Wrap(err, "failed to write document separator")
			}
		}

		if err := yamlSerializer.Encode(doc.object, buf); err != nil {
			return nil, errors.Wrapf(err, "failed to encode document at index %d", doc.index)
		}
	}

	return buf.Bytes(), nil
}

// parseDocument parses a single YAML document and determines if it is convertible.
func parseDocument(trimmed []byte, index int, unstructuredDecoder runtime.Decoder, typedDecoder runtime.Decoder, sourceGVs map[schema.GroupVersion]bool) (document, error) {
	obj := &unstructured.Unstructured{}
	_, gvk, err := unstructuredDecoder.Decode(trimmed, nil, obj)
	if err != nil {
		return document{}, errors.Wrap(err, "failed to decode document: invalid YAML structure")
	}
	if gvk == nil || gvk.Empty() || gvk.Kind == "" || (gvk.Group == "" && gvk.Version == "") {
		return document{}, errors.New("failed to decode document: missing or empty apiVersion/kind")
	}

	convertible := sourceGVs[schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}]

	// Decode into typed object for convertible resources so the conversion machinery works.
	var runtimeObj runtime.Object
	if convertible {
		if typedObj, _, decErr := typedDecoder.Decode(trimmed, gvk, nil); decErr == nil {
			runtimeObj = typedObj
		} else {
			runtimeObj = obj
		}
	} else {
		runtimeObj = obj
	}

	return document{
		object:      runtimeObj,
		gvk:         *gvk,
		convertible: convertible,
		index:       index,
	}, nil
}
