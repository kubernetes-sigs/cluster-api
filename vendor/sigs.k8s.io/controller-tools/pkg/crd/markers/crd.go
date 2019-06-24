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

package markers

import (
	"fmt"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"sigs.k8s.io/controller-tools/pkg/markers"
)

// CRDMarkers lists all markers that directly modify the CRD (not validation
// schemas).
var CRDMarkers = []*markers.Definition{
	markers.Must(markers.MakeDefinition("kubebuilder:subresource:status", markers.DescribesType, SubresourceStatus{})),
	markers.Must(markers.MakeDefinition("kubebuilder:subresource:scale", markers.DescribesType, SubresourceScale{})),
	markers.Must(markers.MakeDefinition("kubebuilder:printcolumn", markers.DescribesType, PrintColumn{})),
	markers.Must(markers.MakeDefinition("kubebuilder:resource", markers.DescribesType, Resource{})),
	markers.Must(markers.MakeDefinition("kubebuilder:storageversion", markers.DescribesType, StorageVersion{})),
}

// TODO: categories and singular used to be annotations types
// TODO: doc

func init() {
	AllDefinitions = append(AllDefinitions, CRDMarkers...)
}

// SubresourceStatus defines "+kubebuilder:subresource:status"
type SubresourceStatus struct{}

func (s SubresourceStatus) ApplyToCRD(crd *apiext.CustomResourceDefinitionSpec, version string) error {
	var subresources *apiext.CustomResourceSubresources
	if version == "" {
		// single-version
		if crd.Subresources == nil {
			crd.Subresources = &apiext.CustomResourceSubresources{}
		}
		subresources = crd.Subresources
	} else {
		// multi-version
		for i := range crd.Versions {
			ver := &crd.Versions[i]
			if ver.Name != version {
				continue
			}
			if ver.Subresources == nil {
				ver.Subresources = &apiext.CustomResourceSubresources{}
			}
			subresources = ver.Subresources
			break
		}
	}
	if subresources == nil {
		return fmt.Errorf("status subresource applied to version %q not in CRD", version)
	}
	subresources.Status = &apiext.CustomResourceSubresourceStatus{}
	return nil
}

// SubresourceScale defines "+kubebuilder:subresource:scale"
type SubresourceScale struct {
	// marker names are leftover legacy cruft
	SpecPath     string  `marker:"specpath"`
	StatusPath   string  `marker:"statuspath"`
	SelectorPath *string `marker:"selectorpath"`
}

func (s SubresourceScale) ApplyToCRD(crd *apiext.CustomResourceDefinitionSpec, version string) error {
	var subresources *apiext.CustomResourceSubresources
	if version == "" {
		// single-version
		if crd.Subresources == nil {
			crd.Subresources = &apiext.CustomResourceSubresources{}
		}
		subresources = crd.Subresources
	} else {
		// multi-version
		for i := range crd.Versions {
			ver := &crd.Versions[i]
			if ver.Name != version {
				continue
			}
			if ver.Subresources == nil {
				ver.Subresources = &apiext.CustomResourceSubresources{}
			}
			subresources = ver.Subresources
			break
		}
	}
	if subresources == nil {
		return fmt.Errorf("scale subresource applied to version %q not in CRD", version)
	}
	subresources.Scale = &apiext.CustomResourceSubresourceScale{
		SpecReplicasPath:   s.SpecPath,
		StatusReplicasPath: s.StatusPath,
		LabelSelectorPath:  s.SelectorPath,
	}
	return nil
}

// StorageVersion defines "+kubebuilder:storageversion"
type StorageVersion struct{}

func (s StorageVersion) ApplyToCRD(crd *apiext.CustomResourceDefinitionSpec, version string) error {
	if version == "" {
		// single-version, do nothing
		return nil
	}
	// multi-version
	for i := range crd.Versions {
		ver := &crd.Versions[i]
		if ver.Name != version {
			continue
		}
		ver.Storage = true
		break
	}
	return nil
}

// PrintColumn defines "+kubebuilder:printcolumn"
type PrintColumn struct {
	Name        string
	Type        string
	JSONPath    string `marker:"JSONPath"` // legacy cruft
	Description string `marker:",optional"`
	Format      string `marker:",optional"`
	Priority    int32  `marker:",optional"`
}

func (s PrintColumn) ApplyToCRD(crd *apiext.CustomResourceDefinitionSpec, version string) error {
	var columns *[]apiext.CustomResourceColumnDefinition
	if version == "" {
		columns = &crd.AdditionalPrinterColumns
	} else {
		for i := range crd.Versions {
			ver := &crd.Versions[i]
			if ver.Name != version {
				continue
			}
			if ver.Subresources == nil {
				ver.Subresources = &apiext.CustomResourceSubresources{}
			}
			columns = &ver.AdditionalPrinterColumns
			break
		}
	}
	if columns == nil {
		return fmt.Errorf("printer columns applied to version %q not in CRD", version)
	}

	*columns = append(*columns, apiext.CustomResourceColumnDefinition{
		Name:        s.Name,
		Type:        s.Type,
		JSONPath:    s.JSONPath,
		Description: s.Description,
		Format:      s.Format,
		Priority:    s.Priority,
	})

	return nil
}

// Resource defines "+kubebuilder:resource"
type Resource struct {
	Path       string
	ShortName  []string `marker:",optional"`
	Categories []string `marker:",optional"`
	Singular   string   `marker:",optional"`
	Scope      string   `marker:",optional"`
}

func (s Resource) ApplyToCRD(crd *apiext.CustomResourceDefinitionSpec, version string) error {
	crd.Names.Plural = s.Path
	crd.Names.ShortNames = s.ShortName
	crd.Names.Categories = s.Categories

	switch s.Scope {
	case "":
		crd.Scope = apiext.NamespaceScoped
	default:
		crd.Scope = apiext.ResourceScope(s.Scope)
	}

	return nil
}

// NB(directxman12): singular was historically distinct, so we keep it here for backwards compat
