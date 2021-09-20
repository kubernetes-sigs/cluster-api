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

package cloudinit

import (
	"fmt"
	"testing"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/yaml"
)

func TestNewNode(t *testing.T) {
	tests := []struct {
		name    string
		input   *NodeInput
		check   func([]byte) error
		wantErr bool
	}{
		{
			"check for duplicated write_files",
			&NodeInput{
				BaseUserData: BaseUserData{
					AdditionalFiles: []bootstrapv1.File{
						{
							Path:        "/etc/foo.conf",
							Content:     "bar",
							Owner:       "root",
							Permissions: "0644",
						},
					},
				},
			},
			checkWriteFiles("/etc/foo.conf", "/run/kubeadm/kubeadm-join-config.yaml", "/run/cluster-api/placeholder"),
			false,
		},
		{
			"check for existence of /run/kubeadm/kubeadm-join-config.yaml and /run/cluster-api/placeholder",
			&NodeInput{},
			checkWriteFiles("/run/kubeadm/kubeadm-join-config.yaml", "/run/cluster-api/placeholder"),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err := tt.check(got); err != nil {
				t.Errorf("%v: got = %s", err, got)
			}
		})
	}
}

func checkWriteFiles(files ...string) func(b []byte) error {
	return func(b []byte) error {
		var cloudinitData struct {
			WriteFiles []struct {
				Path string `json:"path"`
			} `json:"write_files"`
		}

		if err := yaml.Unmarshal(b, &cloudinitData); err != nil {
			return err
		}

		gotFiles := map[string]bool{}
		for _, f := range cloudinitData.WriteFiles {
			gotFiles[f.Path] = true
		}
		for _, file := range files {
			if !gotFiles[file] {
				return fmt.Errorf("expected %q to exist in CloudInit's write_files", file)
			}
		}
		if len(files) != len(cloudinitData.WriteFiles) {
			return fmt.Errorf("expected to have %d files generated to CloudInit's write_files, got %d", len(files), len(cloudinitData.WriteFiles))
		}

		return nil
	}
}
