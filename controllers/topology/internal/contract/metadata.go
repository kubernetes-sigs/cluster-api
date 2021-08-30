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

package contract

import (
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

// Metadata provides a helper struct for working with Metadata.
type Metadata struct {
	path Path
}

// Path returns the path of the metadata.
func (m *Metadata) Path() Path {
	return m.path
}

// Get gets the metadata object.
func (m *Metadata) Get(obj *unstructured.Unstructured) (*clusterv1.ObjectMeta, error) {
	labelsPath := append(m.path, "labels")
	labelsValue, ok, err := unstructured.NestedStringMap(obj.UnstructuredContent(), labelsPath...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve control plane metadata.labels")
	}
	if !ok {
		return nil, errors.Errorf("%s not found", "."+strings.Join(labelsPath, "."))
	}

	annotationsPath := append(m.path, "annotations")
	annotationsValue, ok, err := unstructured.NestedStringMap(obj.UnstructuredContent(), annotationsPath...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve control plane metadata.annotations")
	}
	if !ok {
		return nil, errors.Errorf("%s not found", "."+strings.Join(annotationsPath, "."))
	}

	return &clusterv1.ObjectMeta{
		Labels:      labelsValue,
		Annotations: annotationsValue,
	}, nil
}

// Set sets the metadata value.
func (m *Metadata) Set(obj *unstructured.Unstructured, metadata *clusterv1.ObjectMeta) error {
	labelsPath := append(m.path, "labels")
	if err := unstructured.SetNestedStringMap(obj.UnstructuredContent(), metadata.Labels, labelsPath...); err != nil {
		return errors.Wrap(err, "failed to set control plane metadata.labels")
	}

	annotationsPath := append(m.path, "annotations")
	if err := unstructured.SetNestedStringMap(obj.UnstructuredContent(), metadata.Annotations, annotationsPath...); err != nil {
		return errors.Wrap(err, "failed to set control plane metadata.annotations")
	}
	return nil
}
