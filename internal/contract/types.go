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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var errNotFound = errors.New("not found")

// Path defines a how to access a field in an Unstructured object.
type Path []string

// Int64 represents an accessor to an int64 path value.
type Int64 struct {
	path Path
}

// Path returns the path to the int64 value.
func (i *Int64) Path() Path {
	return i.path
}

// Get gets the int64 value.
func (i *Int64) Get(obj *unstructured.Unstructured) (*int64, error) {
	value, ok, err := unstructured.NestedInt64(obj.UnstructuredContent(), i.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(i.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(errNotFound, "path %s", "."+strings.Join(i.path, "."))
	}
	return &value, nil
}

// Set sets the int64 value in the path.
func (i *Int64) Set(obj *unstructured.Unstructured, value int64) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value, i.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(i.path, "."), obj.GroupVersionKind())
	}
	return nil
}

// String represents an accessor to a string path value.
type String struct {
	path Path
}

// Path returns the path to the string value.
func (s *String) Path() Path {
	return s.path
}

// Get gets the string value.
func (s *String) Get(obj *unstructured.Unstructured) (*string, error) {
	value, ok, err := unstructured.NestedString(obj.UnstructuredContent(), s.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(s.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(errNotFound, "path %s", "."+strings.Join(s.path, "."))
	}
	return &value, nil
}

// Set sets the string value in the path.
func (s *String) Set(obj *unstructured.Unstructured, value string) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value, s.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(s.path, "."), obj.GroupVersionKind())
	}
	return nil
}

// Duration represents an accessor to a metav1.Duration path value.
type Duration struct {
	path Path
}

// Path returns the path to the metav1.Duration value.
func (i *Duration) Path() Path {
	return i.path
}

// Get gets the metav1.Duration value.
func (i *Duration) Get(obj *unstructured.Unstructured) (*metav1.Duration, error) {
	durationString, ok, err := unstructured.NestedString(obj.UnstructuredContent(), i.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(i.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(errNotFound, "path %s", "."+strings.Join(i.path, "."))
	}

	d := &metav1.Duration{}
	if err := d.UnmarshalJSON([]byte(durationString)); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal duration %s from object", "."+strings.Join(i.path, "."))
	}

	return d, nil
}

// Set sets the metav1.Duration value in the path.
func (i *Duration) Set(obj *unstructured.Unstructured, value metav1.Duration) error {
	durationString, err := value.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal duration %s", value.Duration.String())
	}

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), string(durationString), i.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(i.path, "."), obj.GroupVersionKind())
	}
	return nil
}
