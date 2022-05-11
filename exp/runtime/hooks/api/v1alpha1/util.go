/*
Copyright 2022 The Kubernetes Authors.

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

package v1alpha1

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

const (
	fieldStatus  = "Status"
	fieldMessage = "Message"
)

func valueOf(obj interface{}) (reflect.Value, error) {
	var v reflect.Value
	var t reflect.Type
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		t = reflect.TypeOf(obj).Elem()
		v = reflect.ValueOf(obj).Elem()
	} else {
		t = reflect.TypeOf(obj)
		v = reflect.ValueOf(obj)
	}
	if t.Kind() != reflect.Struct {
		return v, fmt.Errorf("unexpected type")
	}
	return v, nil
}

// IsFailure returns true if the Status field of obj is
// equal to "Failure".
func IsFailure(obj interface{}) (bool, error) {
	status, err := GetStatus(obj)
	if err != nil {
		return false, err
	}
	return status == ResponseStatusFailure, nil
}

// GetStatus sets the `Status` field from obj.
func GetStatus(obj interface{}) (ResponseStatus, error) {
	st, err := getField(obj, fieldStatus)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s field from object", fieldStatus)
	}
	status, ok := st.(ResponseStatus)
	if !ok {
		return "", errors.Wrap(err, "received value of unexpected type")
	}
	return status, nil
}

// SetStatus sets the `Status` field in obj.
func SetStatus(obj interface{}, status ResponseStatus) error {
	return setField(obj, fieldStatus, reflect.ValueOf(status))
}

// GetMessage gets the `Message` field from obj.
func GetMessage(obj interface{}) (string, error) {
	msg, err := getField(obj, fieldMessage)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s field from object", fieldMessage)
	}
	message, ok := msg.(string)
	if !ok {
		return "", errors.Wrap(err, "received value of unexpected type")
	}
	return message, nil
}

// SetMessage sets the `Message` field in obj.
func SetMessage(obj interface{}, msg string) error {
	return setField(obj, fieldMessage, reflect.ValueOf(msg))
}

func getField(obj interface{}, field string) (interface{}, error) {
	v, err := valueOf(obj)
	if err != nil {
		return "", err
	}
	f := v.FieldByName(field)
	if !f.IsValid() {
		return "", fmt.Errorf("unexpected type. filed %s does not exist", field)
	}
	return f.Interface(), nil
}

func setField(obj interface{}, field string, value reflect.Value) error {
	v, err := valueOf(obj)
	if err != nil {
		return err
	}
	f := v.FieldByName(field)
	if !f.CanSet() {
		return fmt.Errorf("unexpected type. cannot set field %s", field)
	}
	f.Set(value)
	return nil
}
