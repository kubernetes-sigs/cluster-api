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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/log"
)

// MetadataClient has methods to work with metadata hosted on a provider repository.
// Metadata are yaml files providing additional information about provider's assets like e.g the version compatibility Matrix.
type MetadataClient interface {
	// Get returns the provider's metadata.
	Get() (*clusterctlv1.Metadata, error)
}

// metadataClient implements MetadataClient.
type metadataClient struct {
	provider   config.Provider
	version    string
	repository Repository
}

// ensure metadataClient implements MetadataClient.
var _ MetadataClient = &metadataClient{}

// newMetadataClient returns a metadataClient.
func newMetadataClient(provider config.Provider, version string, repository Repository) *metadataClient {
	return &metadataClient{
		provider:   provider,
		version:    version,
		repository: repository,
	}
}

func (f *metadataClient) Get() (*clusterctlv1.Metadata, error) {
	log := logf.Log

	// gets the metadata file from the repository
	version := f.version
	name := "metadata.yaml"

	file, err := getLocalOverride(f.provider, version, name)
	if err != nil {
		return nil, err
	}
	if file == nil {
		log.V(1).Info("Fetching", "File", name, "Provider", f.provider.Name(), "Version", version)
		file, err = f.repository.GetFile(version, name)
		if err != nil {
			// if there are problems in reading the metadata file from the repository, check if there are embedded metadata for the provider, if yes use them
			if obj := f.getEmbeddedMetadata(); obj != nil {
				return obj, nil
			}

			return nil, errors.Wrapf(err, "failed to read %q from the repository for provider %q", name, f.provider.Name())
		}
	} else {
		log.V(1).Info("Using", "Override", name, "Provider", f.provider.Name(), "Version", version)
	}

	// Convert the yaml into a typed object
	obj := &clusterctlv1.Metadata{}
	codecFactory := serializer.NewCodecFactory(scheme.Scheme)

	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), file, obj); err != nil {
		return nil, errors.Wrapf(err, "error decoding %q for provider %q", name, f.provider.Name())
	}

	//TODO: consider if to add metadata validation (TBD)

	return obj, nil
}

func (f *metadataClient) getEmbeddedMetadata() *clusterctlv1.Metadata {
	// clusterctl includes hard-coded metadata for cluster-API providers developed as a SIG-cluster-lifecycle project in order to
	// provide an option for simplifying the release process/the repository management of those projects.
	// Embedding metadata in clusterctl is optional, and the metadata.yaml file on the provider repository will always take precedence
	// on the embedded one.

	// if you are a developer of a SIG-cluster-lifecycle project, you can send a PR to extend the following list.
	switch f.provider.Name() {
	case config.ClusterAPIProviderName:
		return &clusterctlv1.Metadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterctlv1.GroupVersion.String(),
				Kind:       "Metadata",
			},
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				// v1alpha3 release series
				{Major: 0, Minor: 3, ClusterAPIVersion: "v1alpha3"},
				// v1alpha2 release series are supported only for upgrades
				{Major: 0, Minor: 2, ClusterAPIVersion: "v1alpha2"},
				// older version are not supported by clusterctl
			},
		}
	case config.DockerProviderName:
		return &clusterctlv1.Metadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterctlv1.GroupVersion.String(),
				Kind:       "Metadata",
			},
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				// v1alpha3 release series
				{Major: 0, Minor: 3, ClusterAPIVersion: "v1alpha3"}, // From this release series CAPD version scheme is linked to CAPI
				// v1alpha2 release series are supported only for upgrades
				{Major: 0, Minor: 2, ClusterAPIVersion: "v1alpha2"}, // This release was hosted on a different repository
				// older version are not supported by clusterctl
			},
		}
	case config.KubeadmBootstrapProviderName:
		return &clusterctlv1.Metadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterctlv1.GroupVersion.String(),
				Kind:       "Metadata",
			},
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				// v1alpha3 release series
				{Major: 0, Minor: 3, ClusterAPIVersion: "v1alpha3"}, // From this release series CABPK version scheme is linked to CAPI; The 0.2 release series was skipped when doing this change.
				// v1alpha2 release series are supported only for upgrades
				{Major: 0, Minor: 1, ClusterAPIVersion: "v1alpha2"}, // This release was hosted on a different repository
				// older version are not supported by clusterctl
			},
		}
	case config.AWSProviderName:
		return &clusterctlv1.Metadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterctlv1.GroupVersion.String(),
				Kind:       "Metadata",
			},
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				// v1alpha3 release series
				{Major: 0, Minor: 5, ClusterAPIVersion: "v1alpha3"},
				// v1alpha2 release series are supported only for upgrades
				{Major: 0, Minor: 4, ClusterAPIVersion: "v1alpha2"},
				// older version are not supported by clusterctl
			},
		}
	case config.VSphereProviderName:
		return &clusterctlv1.Metadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterctlv1.GroupVersion.String(),
				Kind:       "Metadata",
			},
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				// v1alpha3 release series
				{Major: 0, Minor: 6, ClusterAPIVersion: "v1alpha3"},
				// v1alpha2 release series are supported only for upgrades
				{Major: 0, Minor: 5, ClusterAPIVersion: "v1alpha2"},
				// older version are not supported by clusterctl
			},
		}
	default:
		return nil
	}
}
