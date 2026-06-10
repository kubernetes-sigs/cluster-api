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

package v1beta2

import (
	"reflect"
	"testing"
)

func TestConvertArgs(t *testing.T) {
	argList := []Arg{
		{
			Name:  "foo",
			Value: new("1"),
		},
		{
			Name:  "bar",
			Value: new("1"),
		},
		{
			Name:  "foo",
			Value: new("2"),
		},
	}
	argMap := ConvertFromArgs(argList)

	argList = ConvertToArgs(argMap)
	if len(argList) != 2 {
		t.Fatal("Expected list to have 2 elements")
	}
	expectedArgList := []Arg{
		{
			Name:  "bar",
			Value: new("1"),
		},
		{
			Name:  "foo",
			Value: new("2"),
		},
	}
	if !reflect.DeepEqual(argList, expectedArgList) {
		t.Fatalf("Expected %+v to equal %+v", argList, expectedArgList)
	}
}
