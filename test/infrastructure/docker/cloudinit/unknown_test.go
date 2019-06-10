/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"reflect"
	"testing"
)

func TestUnknown_Run(t *testing.T) {
	u := &unknown{
		lines: []string{},
	}
	lines, err := u.Run(nil)
	if err != nil {
		t.Fatal("err should always be nil")
	}
	if len(lines) != 0 {
		t.Fatalf("return exactly what was parsed. did not expect anything here but got: %v", lines)
	}
}

func TestUnknown_Unmarshal(t *testing.T) {
	u := &unknown{}
	expected := []string{"test 1", "test 2", "test 3"}
	input := `["test 1", "test 2", "test 3"]`
	if err := u.Unmarshal([]byte(input)); err != nil {
		t.Fatalf("should not return an error but got: %v", err)
	}
	if len(u.lines) != len(expected) {
		t.Fatalf("expected exactly %v lines but got %v", 3, u.lines)
	}
	if !reflect.DeepEqual(u.lines, expected) {
		t.Fatalf("lines should be %v but it is %v", expected, u.lines)
	}
}
