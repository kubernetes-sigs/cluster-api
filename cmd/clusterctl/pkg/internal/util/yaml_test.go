/*
Copyright 2020 The Kubernetes Authors.

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

package util

import (
	"reflect"
	"testing"
)

func TestToUnstructured(t *testing.T) {
	type args struct {
		rawyaml []byte
	}
	tests := []struct {
		name          string
		args          args
		wantObjsCount int
		wantErr       bool
	}{
		{
			name: "single object",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n"),
			},
			wantObjsCount: 1,
			wantErr:       false,
		},
		{
			name: "multiple objects are detected",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "empty object are dropped",
			args: args{
				rawyaml: []byte("---\n" + //empty objects before
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" + // empty objects in the middle
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n" +
					"---\n" + //empty objects after
					"---\n" +
					"---\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "--- in the middle of objects are ignored",
			args: args{
				[]byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"data: \n" +
					" key: |\n" +
					"  ··Several lines of text,\n" +
					"  ··with some --- \n" +
					"  ---\n" +
					"  ··in the middle\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToUnstructured(tt.args.rawyaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(len(got), tt.wantObjsCount) {
				t.Errorf("got = %v object, want %v", len(got), tt.wantObjsCount)
			}
		})
	}
}
