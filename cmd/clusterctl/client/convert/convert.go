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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

// SupportedTargetVersions defines all supported target API versions for conversion.
var SupportedTargetVersions = []string{
	clusterv1.GroupVersion.Version,
}

// Converter handles the conversion of CAPI core resources between API versions.
type Converter struct {
	scheme              *runtime.Scheme
	targetAPIGroup      string
	targetGV            schema.GroupVersion
	sourceGroupVersions []schema.GroupVersion
	knownAPIGroups      []string
}

// NewConverter creates a new Converter instance.
func NewConverter(targetAPIGroup string, targetGV schema.GroupVersion, sourceGroupVersions []schema.GroupVersion, knownAPIGroups []string) *Converter {
	return &Converter{
		scheme:              scheme.Scheme,
		targetAPIGroup:      targetAPIGroup,
		targetGV:            targetGV,
		sourceGroupVersions: sourceGroupVersions,
		knownAPIGroups:      knownAPIGroups,
	}
}

// Convert processes multi-document YAML streams and converts resources to the target version.
func (c *Converter) Convert(input []byte, toVersion string) (output []byte, messages []string, err error) {
	messages = make([]string, 0)

	targetGV := schema.GroupVersion{
		Group:   c.targetAPIGroup,
		Version: toVersion,
	}

	// Create GVK matcher for resource classification.
	matcher := newGVKMatcher(c.sourceGroupVersions, c.knownAPIGroups)

	// Parse input YAML stream.
	docs, err := parseYAMLStream(input, c.scheme, matcher)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse YAML stream")
	}

	for i := range docs {
		doc := &docs[i]

		switch doc.typ {
		case resourceTypeConvertible:
			convertedObj, wasConverted, convErr := convertResource(doc.object, targetGV, c.scheme, c.targetAPIGroup)
			if convErr != nil {
				return nil, nil, errors.Wrapf(convErr, "failed to convert resource %s at index %d", doc.gvk.String(), doc.index)
			}

			if wasConverted {
				doc.object = convertedObj
			} else {
				// Resource that are already at target version.
				if msg := getInfoMessage(doc.gvk, toVersion, c.targetAPIGroup); msg != "" {
					messages = append(messages, msg)
				}
			}

		case resourceTypeKnown:
			// Pass through unchanged with info message.
			if msg := getInfoMessage(doc.gvk, toVersion, c.targetAPIGroup); msg != "" {
				messages = append(messages, msg)
			}

		case resourceTypePassThrough:
			// Non-target API group resource - pass through unchanged with info message.
			if msg := getInfoMessage(doc.gvk, toVersion, c.targetAPIGroup); msg != "" {
				messages = append(messages, msg)
			}
		}
	}

	// Serialize documents back to YAML.
	output, err = serializeYAMLStream(docs, c.scheme)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to serialize output")
	}

	return output, messages, nil
}
