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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ErrFieldNotFound is returned when a field is not found in the object.
var ErrFieldNotFound = errors.New("field not found")

// Path defines a how to access a field in an Unstructured object.
type Path []string

// Append a field name to a path.
func (p Path) Append(k string) Path {
	return append(p, k)
}

// IsParentOf check if one path is Parent of the other.
func (p Path) IsParentOf(other Path) bool {
	if len(p) >= len(other) {
		return false
	}
	for i := range p {
		if p[i] != other[i] {
			return false
		}
	}
	return true
}

// Equal check if two path are equal (exact match).
func (p Path) Equal(other Path) bool {
	if len(p) != len(other) {
		return false
	}
	for i := range p {
		if p[i] != other[i] {
			return false
		}
	}
	return true
}

// Overlaps return true if two paths are Equal or one IsParentOf the other.
func (p Path) Overlaps(other Path) bool {
	return other.Equal(p) || other.IsParentOf(p) || p.IsParentOf(other)
}

// String returns the path as a dotted string.
func (p Path) String() string {
	return strings.Join(p, ".")
}

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
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(i.path, "."))
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

// Int32 represents an accessor to an int32 path value.
type Int32 struct {
	paths []Path
}

// Path returns the path to the int32 value.
func (i *Int32) Path() []Path {
	return i.paths
}

// Get gets the int32 value.
// If more than one path is defined, path are checked in order and the first value reported is used
// (e.g. in case when we are reading from a preferred location first, and then from one or more fallback locations).
func (i *Int32) Get(obj *unstructured.Unstructured) (*int32, error) {
	paths := []string{}
	for _, path := range i.paths {
		value, ok, err := unstructured.NestedInt64(obj.UnstructuredContent(), path...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(path, "."))
		}
		if !ok {
			paths = append(paths, "."+strings.Join(path, "."))
			continue
		}
		int32Value := int32(value)
		return &int32Value, nil
	}
	return nil, errors.Wrapf(ErrFieldNotFound, "path %s", strings.Join(paths, ", "))
}

// Set sets the int32 value in the path.
// If more than one path is defined, only the first path will be set.
// (the first path is considered the preferred location).
// Note: Cluster API should never Set values on external objects owner by providers; however this method is useful for writing tests.
func (i *Int32) Set(obj *unstructured.Unstructured, value int64) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value, i.paths[0]...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(i.paths[0], "."), obj.GroupVersionKind())
	}
	return nil
}

// Bool represents an accessor to an bool path value.
type Bool struct {
	path Path
}

// Path returns the path to the bool value.
func (b *Bool) Path() Path {
	return b.path
}

// Get gets the bool value.
func (b *Bool) Get(obj *unstructured.Unstructured) (*bool, error) {
	value, ok, err := unstructured.NestedBool(obj.UnstructuredContent(), b.path...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s from object", "."+strings.Join(b.path, "."))
	}
	if !ok {
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(b.path, "."))
	}
	return &value, nil
}

// Set sets the bool value in the path.
func (b *Bool) Set(obj *unstructured.Unstructured, value bool) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value, b.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(b.path, "."), obj.GroupVersionKind())
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
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(s.path, "."))
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
		return nil, errors.Wrapf(ErrFieldNotFound, "path %s", "."+strings.Join(i.path, "."))
	}

	d := &metav1.Duration{}
	if err := d.UnmarshalJSON([]byte(strconv.Quote(durationString))); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal duration %s from object", "."+strings.Join(i.path, "."))
	}

	return d, nil
}

// Set sets the metav1.Duration value in the path.
func (i *Duration) Set(obj *unstructured.Unstructured, value metav1.Duration) error {
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), value.Duration.String(), i.path...); err != nil {
		return errors.Wrapf(err, "failed to set path %s of object %v", "."+strings.Join(i.path, "."), obj.GroupVersionKind())
	}
	return nil
}
