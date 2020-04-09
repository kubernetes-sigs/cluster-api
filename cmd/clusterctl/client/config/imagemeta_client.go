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

package config

import (
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/container"
)

const (
	imagesConfigKey = "images"
	allImageConfig  = "all"
)

// ImageMetaClient has methods to work with image meta configurations.
type ImageMetaClient interface {
	// AlterImage alters an image name according to the current image override configurations.
	AlterImage(component, image string) (string, error)
}

// imageMetaClient implements ImageMetaClient.
type imageMetaClient struct {
	reader         Reader
	imageMetaCache map[string]*imageMeta
}

// ensure imageMetaClient implements ImageMetaClient.
var _ ImageMetaClient = &imageMetaClient{}

func newImageMetaClient(reader Reader) *imageMetaClient {
	return &imageMetaClient{
		reader:         reader,
		imageMetaCache: map[string]*imageMeta{},
	}
}

func (p *imageMetaClient) AlterImage(component, image string) (string, error) {
	// Gets the image meta that applies to the selected component; if none, returns early
	meta, err := p.getImageMetaByComponent(component)
	if err != nil {
		return "", err
	}
	if meta == nil {
		return image, nil
	}

	// Apply the image meta to image name
	return meta.ApplyToImage(image)
}

// getImageMetaByComponent returns the image meta that applies to the selected component
func (p *imageMetaClient) getImageMetaByComponent(component string) (*imageMeta, error) {
	// if the image meta for the component is already known, return it
	if im, ok := p.imageMetaCache[component]; ok {
		return im, nil
	}

	// Otherwise read the image override configurations.
	var meta map[string]imageMeta
	if err := p.reader.UnmarshalKey(imagesConfigKey, &meta); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal image override configurations")
	}

	// If there are not image override configurations, return.
	if meta == nil {
		p.imageMetaCache[component] = nil
		return nil, nil
	}

	// Gets the image configuration and to the specific component, and returns the union of the two.
	m := &imageMeta{}
	if allMeta, ok := meta[allImageConfig]; ok {
		m.Union(&allMeta)
	}
	if componentMeta, ok := meta[component]; ok {
		m.Union(&componentMeta)
	}

	p.imageMetaCache[component] = m
	return m, nil
}

// imageMeta allows to define transformations to apply to the image contained in the YAML manifests.
type imageMeta struct {
	// repository sets the container registry to pull images from.
	Repository string `json:"repository,omitempty"`

	// Tag allows to specify a tag for the images.
	Tag string `json:"tag,omitempty"`
}

// Union allows to merge two imageMeta transformation; in case both the imageMeta defines new values for the same field,
// the other transformation takes precedence on the existing one.
func (i *imageMeta) Union(other *imageMeta) {
	if other.Repository != "" {
		i.Repository = other.Repository
	}
	if other.Tag != "" {
		i.Tag = other.Tag
	}
}

// ApplyToImage changes an image name applying the transformations defined in the current imageMeta.
func (i *imageMeta) ApplyToImage(image string) (string, error) {

	newImage, err := container.ImageFromString(image)
	if err != nil {
		return "", err
	}

	// apply transformations
	if i.Repository != "" {
		newImage.Repository = strings.TrimSuffix(i.Repository, "/")
	}
	if i.Tag != "" {
		newImage.Tag = i.Tag
	}

	// returns the resulting image name
	return newImage.String(), nil
}
