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

package jsonfs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/state"
)

type JSONFileSystemStoreOptions struct {
	BasePath    string
	ClusterName string
}

// JSONFileSystemStore exists to save the cluster at runtime to the file defined
// in the state.ClusterJSONFile constant. We perform this operation so that
// various bash scripts can get the cluster state at runtime without having to
// inject key/value pairs into the script or anything like that.
type JSONFileSystemStore struct {
	options      *JSONFileSystemStoreOptions
	ClusterName  string
	BasePath     string
	AbsolutePath string
}

func NewJSONFileSystemStore(o *JSONFileSystemStoreOptions) *JSONFileSystemStore {
	return &JSONFileSystemStore{
		options:      o,
		ClusterName:  o.ClusterName,
		BasePath:     o.BasePath,
		AbsolutePath: filepath.Join(o.BasePath, o.ClusterName),
	}
}

func (fs *JSONFileSystemStore) Exists() bool {
	if _, err := os.Stat(fs.AbsolutePath); os.IsNotExist(err) {
		return false
	}
	return true
}

func (fs *JSONFileSystemStore) write(relativePath string, data []byte) error {
	fqn := filepath.Join(fs.AbsolutePath, relativePath)
	err := os.MkdirAll(path.Dir(fqn), 0700)
	if err != nil {
		return err
	}
	fo, err := os.Create(fqn)
	if err != nil {
		return err
	}
	defer fo.Close()
	_, err = io.Copy(fo, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	return nil
}

func (fs *JSONFileSystemStore) Read(relativePath string) ([]byte, error) {
	fqn := filepath.Join(fs.AbsolutePath, relativePath)
	bytes, err := ioutil.ReadFile(fqn)
	if err != nil {
		return []byte(""), err
	}
	return bytes, nil
}

func (fs *JSONFileSystemStore) ReadStore() ([]byte, error) {
	return fs.Read(state.ClusterJSONFile)
}

func (fs *JSONFileSystemStore) Commit(c *cluster.Cluster) error {
	if c == nil {
		return fmt.Errorf("Nil cluster spec")
	}
	bytes, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return fs.write(state.ClusterJSONFile, bytes)
}

func (fs *JSONFileSystemStore) Rename(existingRelativePath, newRelativePath string) error {
	return os.Rename(existingRelativePath, newRelativePath)
}

func (fs *JSONFileSystemStore) Destroy() error {
	logger.Warning("Removing path [%s]", fs.AbsolutePath)
	return os.RemoveAll(fs.AbsolutePath)
}

func (fs *JSONFileSystemStore) GetCluster() (*cluster.Cluster, error) {
	configBytes, err := fs.Read(state.ClusterJSONFile)
	if err != nil {
		return nil, err
	}

	return fs.BytesToCluster(configBytes)
}

func (fs *JSONFileSystemStore) BytesToCluster(bytes []byte) (*cluster.Cluster, error) {
	cluster := &cluster.Cluster{}
	err := json.Unmarshal(bytes, cluster)
	if err != nil {
		return cluster, err
	}
	return cluster, nil
}

func (fs *JSONFileSystemStore) List() ([]string, error) {

	var stateList []string

	files, err := ioutil.ReadDir(fs.AbsolutePath)
	if err != nil {
		return stateList, err
	}

	for _, file := range files {
		stateList = append(stateList, file.Name())
	}

	return stateList, nil
}
