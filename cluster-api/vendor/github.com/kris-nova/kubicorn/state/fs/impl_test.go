// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/profiles/amazon"
	"github.com/kris-nova/kubicorn/state"
)

func TestStateFileSystem(t *testing.T) {
	testFilePath := ".test"
	clusterName := "fstest"

	c := amazon.NewUbuntuCluster(clusterName)
	o := &FileSystemStoreOptions{
		BasePath:    testFilePath,
		ClusterName: c.Name,
	}
	fs := NewFileSystemStore(o)
	if err := fs.Destroy(); err != nil {
		t.Fatalf("Error destroying any existing state: %v", err)
	}
	if fs.Exists() {
		t.Fatalf("State shouldn't exist because we just destroyed it, but Exists() returned true")
	}
	if err := fs.Commit(c); err != nil {
		t.Fatalf("Error committing cluster: %v", err)
	}
	files, err := fs.List()
	if err != nil {
		t.Fatalf("Error listing files: %v", err)
	}
	if len(files) < 1 {
		t.Fatalf("Expected at least one cluster, got: %v", len(files))
	}

	if filepath.Join(files[0], "cluster.yaml") != filepath.Join(clusterName, state.ClusterYamlFile) {
		t.Fatalf("Expected file name to be %v, got %v", state.ClusterYamlFile, files[0])
	}
	read, err := fs.GetCluster()
	if err != nil {
		t.Fatalf("Error getting cluster: %v", err)
	}
	if !reflect.DeepEqual(read, c) {
		t.Fatalf("Cluster in doesn't equal cluster out")
	}
	unmarshalled := &cluster.Cluster{}
	bytes, err := ioutil.ReadFile(filepath.Join(testFilePath, clusterName, state.ClusterYamlFile))
	if err != nil {
		t.Fatalf("Error reading yaml file: %v", err)
	}
	if err := yaml.Unmarshal(bytes, unmarshalled); err != nil {
		t.Fatalf("Error unmarshalling yaml: %v", err)
	}
	if !reflect.DeepEqual(unmarshalled, c) {
		t.Fatalf("Cluster read directly from yaml file doesn't equal cluster inputted: %v", unmarshalled)
	}
	if err = fs.Destroy(); err != nil {
		t.Fatalf("Error cleaning up state: %v", err)
	}
}
