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

package repository

import (
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

// MemoryRepository contains an instance of the repository data.
type MemoryRepository struct {
	defaultVersion string
	rootPath       string
	componentsPath string
	versions       map[string]bool
	files          map[string][]byte
}

var _ Repository = &MemoryRepository{}

// NewMemoryRepository returns a new MemoryRepository instance.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		versions: map[string]bool{},
		files:    map[string][]byte{},
	}
}

// DefaultVersion returns the default version for this repository.
// NOTE: The DefaultVersion is a required info usually derived from the repository configuration,
// and it is used whenever the users gets files from the repository without providing a specific version.
func (f *MemoryRepository) DefaultVersion() string {
	if f.defaultVersion == "" {
		return latestVersionTag
	}
	return f.defaultVersion
}

// RootPath returns the RootPath for this repository.
// NOTE: The RootPath is a required info usually derived from the repository configuration,
// and it is used to map the file path to the internal repository structure.
func (f *MemoryRepository) RootPath() string {
	return f.rootPath
}

// ComponentsPath returns ComponentsPath for this repository
// NOTE: The ComponentsPath is a required info usually derived from the repository configuration,
// and it is used to identify the components yaml for the provider.
func (f *MemoryRepository) ComponentsPath() string {
	return f.componentsPath
}

// GetFile returns a file for a given provider version.
// NOTE: If the provided version is missing, the default version is used.
func (f *MemoryRepository) GetFile(version string, path string) ([]byte, error) {
	if version == "" {
		version = f.DefaultVersion()
	}
	if version == latestVersionTag {
		var err error
		version, err = latestContractRelease(f, clusterv1.GroupVersion.Version)
		if err != nil {
			return nil, err
		}
	}
	if _, ok := f.versions[version]; !ok {
		return nil, errors.Errorf("unable to get files for version %s", version)
	}

	for p, c := range f.files {
		if p == vpath(version, path) {
			return c, nil
		}
	}
	return nil, errors.Errorf("unable to get file %s for version %s", path, version)
}

// GetVersions returns the list of versions that are available.
func (f *MemoryRepository) GetVersions() ([]string, error) {
	v := make([]string, 0, len(f.versions))
	for k := range f.versions {
		v = append(v, k)
	}
	return v, nil
}

// WithPaths allows setting of the rootPath and componentsPath fields.
func (f *MemoryRepository) WithPaths(rootPath, componentsPath string) *MemoryRepository {
	f.rootPath = rootPath
	f.componentsPath = componentsPath
	return f
}

// WithVersions allows setting of the available versions.
// NOTE: When adding a file to the repository for a specific version, a version
// is automatically generated if missing; this func allows to define versions without any file.
func (f *MemoryRepository) WithVersions(version ...string) *MemoryRepository {
	for _, v := range version {
		f.versions[v] = true
	}
	return f
}

// WithDefaultVersion allows setting of the default version.
func (f *MemoryRepository) WithDefaultVersion(version string) *MemoryRepository {
	f.defaultVersion = version
	return f
}

// WithFile allows setting of a file for a given version.
// NOTE:
// - If the provided version is missing, a new one will be generated automatically.
// - If the defaultVersion has not been set, it will be initialized with the first version passed in WithFile().
// - If the version is "latest" or "", nothing will be added.
func (f *MemoryRepository) WithFile(version, path string, content []byte) *MemoryRepository {
	if version == latestVersionTag || version == "" {
		return f
	}

	f.versions[version] = true
	f.files[vpath(version, path)] = content

	if f.defaultVersion == "" {
		f.defaultVersion = version
	}
	return f
}

// WithMetadata allows setting of the metadata.
func (f *MemoryRepository) WithMetadata(version string, metadata *clusterctlv1.Metadata) *MemoryRepository {
	codecs := serializer.NewCodecFactory(scheme.Scheme)

	mediaType := "application/yaml"
	info, match := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), mediaType)
	if !match {
		panic("failed to get SerializerInfo for application/yaml")
	}

	metadata.SetGroupVersionKind(clusterctlv1.GroupVersion.WithKind("Metadata"))

	encoder := codecs.EncoderForVersion(info.Serializer, metadata.GroupVersionKind().GroupVersion())
	data, err := runtime.Encode(encoder, metadata)
	if err != nil {
		panic(err)
	}

	return f.WithFile(version, "metadata.yaml", data)
}

func vpath(version string, path string) string {
	return fmt.Sprintf("%s/%s", version, path)
}
