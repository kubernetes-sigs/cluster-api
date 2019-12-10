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

package test

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type FakeRepository struct {
	defaultVersion string
	rootPath       string
	componentsPath string
	kustomizeDir   string
	versions       map[string]bool
	files          map[string][]byte
}

func (f *FakeRepository) DefaultVersion() string {
	return f.defaultVersion
}

func (f *FakeRepository) RootPath() string {
	return f.rootPath
}

func (f *FakeRepository) ComponentsPath() string {
	return f.componentsPath
}

func (f *FakeRepository) KustomizeDir() string {
	return f.kustomizeDir
}

func (f FakeRepository) GetFiles(version string, path string) (map[string][]byte, error) {
	if _, ok := f.versions[version]; !ok {
		return nil, errors.Errorf("unable to get files for version %s", version)
	}

	ret := map[string][]byte{}
	for p, c := range f.files {
		if p == vpath(version, path) ||
			strings.HasPrefix(p, vpath(version, path)) { // this is a quick & dirty way for identifying files belonging to a path tree

			x := strings.TrimPrefix(p, fmt.Sprintf("%s/", version))
			ret[x] = c
		}
	}
	return ret, nil
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{
		versions: map[string]bool{},
		files:    map[string][]byte{},
	}
}

func (f *FakeRepository) WithPaths(rootPath, componentsPath, kustomizeDir string) *FakeRepository {
	f.rootPath = rootPath
	f.componentsPath = componentsPath
	f.kustomizeDir = kustomizeDir
	return f
}

func (f *FakeRepository) WithDefaultVersion(version string) *FakeRepository {
	f.defaultVersion = version
	return f
}

func (f *FakeRepository) WithFile(version, path string, content []byte) *FakeRepository {
	f.versions[version] = true
	f.files[vpath(version, path)] = content
	return f
}

func vpath(version string, path string) string {
	return fmt.Sprintf("%s/%s", version, path)
}
