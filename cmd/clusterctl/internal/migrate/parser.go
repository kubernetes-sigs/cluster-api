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

package migrate

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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// Document represents a single YAML document with associated metadata.
type Document struct {
	Object runtime.Object
	Raw    []byte
	GVK    schema.GroupVersionKind
	Type   ResourceType
	// Index indicates the document's position in the original stream.
	Index int
}

// ResourceType classifies the type of Kubernetes resource.
type ResourceType int

const (
	// ResourceTypeCoreV1Beta1 identifies v1beta1 core ClusterAPI resources.
	ResourceTypeCoreV1Beta1 ResourceType = iota
	// ResourceTypeOtherCAPI identifies other cluster.x-k8s.io resources (non-v1beta1).
	ResourceTypeOtherCAPI
	// ResourceTypeNonCAPI identifies Kubernetes objects outside of ClusterAPI groups.
	ResourceTypeNonCAPI
	// ResourceTypeUnsupported identifies documents that could not be parsed or classified.
	ResourceTypeUnsupported
)

// YAMLParser parses YAML documents into runtime objects using Kubernetes serializers.
type YAMLParser struct {
	scheme              *runtime.Scheme
	codecFactory        serializer.CodecFactory
	unstructuredDecoder runtime.Decoder
	typedDecoder        runtime.Decoder
	yamlSerializer      runtime.Serializer
}

// NewYAMLParser creates a new YAML parser with the given scheme.
func NewYAMLParser(scheme *runtime.Scheme) *YAMLParser {
	codecFactory := serializer.NewCodecFactory(scheme)

	return &YAMLParser{
		scheme:              scheme,
		codecFactory:        codecFactory,
		unstructuredDecoder: yamlserializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme),
		typedDecoder:        codecFactory.UniversalDeserializer(),
		yamlSerializer: jsonserializer.NewSerializerWithOptions(
			jsonserializer.DefaultMetaFactory,
			scheme,
			scheme,
			jsonserializer.SerializerOptions{Yaml: true, Pretty: false, Strict: false},
		),
	}
}

// ParseYAMLStream parses a multi-document YAML stream into individual documents.
func (p *YAMLParser) ParseYAMLStream(reader io.Reader) ([]Document, error) {
	yamlReader := yamlutil.NewYAMLReader(bufio.NewReader(reader))

	var documents []Document
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

		doc, err := p.parseDocument(trimmed, raw, index)
		if err != nil {
			// Treat parsing failures as unsupported documents but keep raw bytes.
			documents = append(documents, Document{
				Object: nil,
				Raw:    p.ensureTrailingNewline(raw),
				GVK:    schema.GroupVersionKind{},
				Type:   ResourceTypeUnsupported,
				Index:  index,
			})
		} else {
			documents = append(documents, doc)
		}
		index++
	}

	return documents, nil
}

// SerializeYAMLStream writes documents back out as a multi-document YAML stream.
func (p *YAMLParser) SerializeYAMLStream(docs []Document, writer io.Writer) error {
	if len(docs) == 0 {
		return nil
	}

	for i, doc := range docs {
		// Add document separator before each document except the first
		if i > 0 {
			if _, err := io.WriteString(writer, "---\n"); err != nil {
				return errors.Wrap(err, "failed to write document separator")
			}
		}

		if doc.Object != nil {
			buf := &bytes.Buffer{}
			if err := p.yamlSerializer.Encode(doc.Object, buf); err != nil {
				return errors.Wrapf(err, "failed to encode document at index %d", doc.Index)
			}
			if _, err := writer.Write(p.ensureTrailingNewline(buf.Bytes())); err != nil {
				return errors.Wrapf(err, "failed to write document at index %d", doc.Index)
			}
			continue
		}

		if _, err := writer.Write(p.ensureTrailingNewline(doc.Raw)); err != nil {
			return errors.Wrapf(err, "failed to write raw document at index %d", doc.Index)
		}
	}

	return nil
}

func (p *YAMLParser) parseDocument(trimmed []byte, original []byte, index int) (Document, error) {
	obj := &unstructured.Unstructured{}
	_, gvk, err := p.unstructuredDecoder.Decode(trimmed, nil, obj)
	if err != nil || gvk == nil || gvk.Empty() {
		return Document{}, errors.New("unable to decode document")
	}

	resourceType := p.classifyResource(*gvk)

	var runtimeObj runtime.Object
	if resourceType == ResourceTypeCoreV1Beta1 || resourceType == ResourceTypeOtherCAPI {
		if typedObj, _, err := p.typedDecoder.Decode(trimmed, gvk, nil); err == nil {
			runtimeObj = typedObj
		} else {
			runtimeObj = obj
		}
	} else {
		runtimeObj = obj
	}

	return Document{
		Object: runtimeObj,
		Raw:    p.ensureTrailingNewline(original),
		GVK:    *gvk,
		Type:   resourceType,
		Index:  index,
	}, nil
}

func (p *YAMLParser) ensureTrailingNewline(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	if content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	return content
}

func (p *YAMLParser) classifyResource(gvk schema.GroupVersionKind) ResourceType {
	if p.isCoreCAPIGroup(gvk.Group) {
		if gvk.Version == "v1beta1" {
			return ResourceTypeCoreV1Beta1
		}
		return ResourceTypeOtherCAPI
	}
	return ResourceTypeNonCAPI
}

func (p *YAMLParser) isCoreCAPIGroup(group string) bool {
	_, ok := coreCapiGroups[group]
	return ok
}

var coreCapiGroups = map[string]struct{}{
	clusterv1.GroupVersion.Group: {},
}
