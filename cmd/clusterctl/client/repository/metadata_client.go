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

package repository

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

const metadataFile = "metadata.yaml"

// MetadataClient has methods to work with metadata hosted on a provider repository.
// Metadata are yaml files providing additional information about provider's assets like e.g the version compatibility Matrix.
type MetadataClient interface {
	// Get returns the provider's metadata.
	Get() (*clusterctlv1.Metadata, error)
}

// metadataClient implements MetadataClient.
type metadataClient struct {
	configVarClient config.VariablesClient
	provider        config.Provider
	version         string
	repository      Repository
}

// ensure metadataClient implements MetadataClient.
var _ MetadataClient = &metadataClient{}

// newMetadataClient returns a metadataClient.
func newMetadataClient(provider config.Provider, version string, repository Repository, config config.VariablesClient) *metadataClient {
	return &metadataClient{
		configVarClient: config,
		provider:        provider,
		version:         version,
		repository:      repository,
	}
}

func (f *metadataClient) Get() (*clusterctlv1.Metadata, error) {
	log := logf.Log

	// gets the metadata file from the repository
	version := f.version

	file, err := getLocalOverride(&newOverrideInput{
		configVariablesClient: f.configVarClient,
		provider:              f.provider,
		version:               version,
		filePath:              metadataFile,
	})
	if err != nil {
		return nil, err
	}
	if file == nil {
		log.V(5).Info("Fetching", "File", metadataFile, "Provider", f.provider.Name(), "Type", f.provider.Type(), "Version", version)
		file, err = f.repository.GetFile(version, metadataFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q from the repository for provider %q", metadataFile, f.provider.ManifestLabel())
		}
	} else {
		log.V(1).Info("Using", "Override", metadataFile, "Provider", f.provider.ManifestLabel(), "Version", version)
	}

	// Convert the yaml into a typed object
	obj := &clusterctlv1.Metadata{}
	codecFactory := serializer.NewCodecFactory(scheme.Scheme)

	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), file, obj); err != nil {
		return nil, errors.Wrapf(err, "error decoding %q for provider %q", metadataFile, f.provider.ManifestLabel())
	}

	//TODO: consider if to add metadata validation (TBD)

	return obj, nil
}
