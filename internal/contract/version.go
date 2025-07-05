/*
Copyright 2025 The Kubernetes Authors.

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

package contract

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/contract"
)

var (
	// Version is the contract version supported by this Cluster API version.
	// Note: Each Cluster API version supports one contract version, and by convention the contract version matches the current API version.
	Version = clusterv1.GroupVersion.Version
)

// GetCompatibleVersions return the list of contract version compatible with a given contract version.
// NOTE: A contract version might be temporarily compatible with older contract versions e.g. to allow users time to transition to the new API.
// NOTE: The return value must include also the contract version received in input.
func GetCompatibleVersions(contract string) sets.Set[string] {
	compatibleContracts := sets.New(contract)
	// v1beta2 contract is temporarily be compatible with v1beta1 (until v1beta1 is EOL).
	if contract == "v1beta2" {
		compatibleContracts.Insert("v1beta1")
	}
	return compatibleContracts
}

// UpdateReferenceAPIContract takes a client and object reference, queries the API Server for
// the Custom Resource Definition and looks which one is the stored version available.
//
// The object reference passed as input is modified in place if an updated compatible version is found.
// NOTE: This version depends on CRDs being named correctly as defined by contract.CalculateCRDName.
func UpdateReferenceAPIContract(ctx context.Context, c client.Reader, ref *corev1.ObjectReference) error {
	gvk := ref.GroupVersionKind()

	metadata, err := GetGKMetadata(ctx, c, gvk.GroupKind())
	if err != nil {
		return errors.Wrapf(err, "failed to update apiVersion in ref")
	}

	_, chosen, err := GetLatestContractAndAPIVersionFromContract(metadata, Version)
	if err != nil {
		return errors.Wrapf(err, "failed to update apiVersion in ref")
	}

	// Modify the GroupVersionKind with the new version.
	if gvk.Version != chosen {
		gvk.Version = chosen
		ref.SetGroupVersionKind(gvk)
	}

	return nil
}

// GetContractVersion gets the latest compatible contract version from a CRD based on the current contract Version.
func GetContractVersion(ctx context.Context, c client.Reader, gk schema.GroupKind) (string, error) {
	crdMetadata, err := GetGKMetadata(ctx, c, gk)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version for kind %s", gk.Kind)
	}

	contractVersion, _, err := GetLatestContractAndAPIVersionFromContract(crdMetadata, Version)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version for kind %s", gk.Kind)
	}

	return contractVersion, nil
}

// GetAPIVersion gets the latest compatible apiVersion from a CRD based on the current contract Version.
func GetAPIVersion(ctx context.Context, c client.Reader, gk schema.GroupKind) (string, error) {
	crdMetadata, err := GetGKMetadata(ctx, c, gk)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get apiVersion for kind %s", gk.Kind)
	}

	_, version, err := GetLatestContractAndAPIVersionFromContract(crdMetadata, Version)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get apiVersion for kind %s", gk.Kind)
	}

	return schema.GroupVersion{
		Group:   gk.Group,
		Version: version,
	}.String(), nil
}

// GetLatestContractAndAPIVersionFromContract gets the latest compatible contract version and apiVersion from a CRD based on
// the passed in currentContractVersion.
func GetLatestContractAndAPIVersionFromContract(metadata metav1.Object, currentContractVersion string) (string, string, error) {
	if currentContractVersion == "" {
		return "", "", errors.Errorf("current contract version cannot be empty")
	}

	labels := metadata.GetLabels()

	sortedCompatibleContractVersions := kubeAwareAPIVersions(GetCompatibleVersions(currentContractVersion).UnsortedList())
	sort.Sort(sort.Reverse(sortedCompatibleContractVersions))

	for _, contractVersion := range sortedCompatibleContractVersions {
		contractGroupVersion := fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, contractVersion)

		// If there is no label, return early without changing the reference.
		supportedVersions, ok := labels[contractGroupVersion]
		if !ok || supportedVersions == "" {
			continue
		}

		// Pick the latest version in the slice and validate it.
		kubeVersions := kubeAwareAPIVersions(strings.Split(supportedVersions, "_"))
		sort.Sort(kubeVersions)
		return contractVersion, kubeVersions[len(kubeVersions)-1], nil
	}

	return "", "", errors.Errorf("cannot find any versions matching contract versions %q for CRD %v as contract version label(s) are either missing or empty (see https://cluster-api.sigs.k8s.io/developer/providers/contracts/overview.html#api-version-labels)", sortedCompatibleContractVersions, metadata.GetName())
}

// GetContractVersionForVersion gets the contract version for an apiVersion from a CRD.
// The passed in version is only the version part of the apiVersion, not the full apiVersion.
func GetContractVersionForVersion(ctx context.Context, c client.Reader, gk schema.GroupKind, version string) (string, error) {
	crdMetadata, err := GetGKMetadata(ctx, c, gk)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version for version %s for kind %s", version, gk.Kind)
	}

	contractPrefix := fmt.Sprintf("%s/", clusterv1.GroupVersion.Group)
	for labelKey, labelValue := range crdMetadata.GetLabels() {
		if !strings.HasPrefix(labelKey, contractPrefix) {
			continue
		}

		for _, v := range strings.Split(labelValue, "_") {
			if v == version {
				return strings.TrimPrefix(labelKey, contractPrefix), nil
			}
		}
	}

	return "", errors.Errorf("cannot find any contract version matching version %s for CRD %s", version, crdMetadata.GetName())
}

// GetGKMetadata retrieves a CustomResourceDefinition metadata from the API server using partial object metadata.
//
// This function is greatly more efficient than GetCRDWithContract and should be preferred in most cases.
func GetGKMetadata(ctx context.Context, c client.Reader, gk schema.GroupKind) (*metav1.PartialObjectMetadata, error) {
	meta := &metav1.PartialObjectMetadata{}
	meta.SetName(contract.CalculateCRDName(gk.Group, gk.Kind))
	meta.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
	if err := c.Get(ctx, client.ObjectKeyFromObject(meta), meta); err != nil {
		return meta, errors.Wrap(err, "failed to get CustomResourceDefinition metadata")
	}
	return meta, nil
}

// kubeAwareAPIVersions is a sortable slice of kube-like version strings.
//
// Kube-like version strings are starting with a v, followed by a major version,
// optional "alpha" or "beta" strings followed by a minor version (e.g. v1, v2beta1).
// Versions will be sorted based on GA/alpha/beta first and then major and minor
// versions. e.g. v2, v1, v1beta2, v1beta1, v1alpha1.
type kubeAwareAPIVersions []string

func (k kubeAwareAPIVersions) Len() int      { return len(k) }
func (k kubeAwareAPIVersions) Swap(i, j int) { k[i], k[j] = k[j], k[i] }
func (k kubeAwareAPIVersions) Less(i, j int) bool {
	return k8sversion.CompareKubeAwareVersionStrings(k[i], k[j]) < 0
}
