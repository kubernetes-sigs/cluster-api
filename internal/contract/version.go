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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
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
// The object passed as input is modified in place if an updated compatible version is found.
// NOTE: This version depends on CRDs being named correctly as defined by contract.CalculateCRDName.
func UpdateReferenceAPIContract(ctx context.Context, c client.Client, ref *corev1.ObjectReference) error {
	gvk := ref.GroupVersionKind()

	metadata, err := util.GetGVKMetadata(ctx, c, gvk)
	if err != nil {
		return errors.Wrapf(err, "failed to update apiVersion in ref")
	}

	_, chosen, err := getLatestAPIVersionFromContract(metadata, Version)
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

// GetContractVersion get the latest compatible contract from a CRD based on currentContractVersion.
func GetContractVersion(ctx context.Context, c client.Client, gvk schema.GroupVersionKind) (string, error) {
	crdMetadata, err := util.GetGVKMetadata(ctx, c, gvk)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version")
	}

	contractVersion, _, err := getLatestAPIVersionFromContract(crdMetadata, Version)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version")
	}

	return contractVersion, nil
}

// getLatestAPIVersionFromContract returns the latest apiVersion and the latest compatible contract version from labels.
func getLatestAPIVersionFromContract(metadata metav1.Object, currentContractVersion string) (string, string, error) {
	if currentContractVersion == "" {
		return "", "", errors.Errorf("current contract version cannot be empty")
	}

	labels := metadata.GetLabels()

	sortedCompatibleContractVersions := util.KubeAwareAPIVersions(GetCompatibleVersions(currentContractVersion).UnsortedList())
	sort.Sort(sort.Reverse(sortedCompatibleContractVersions))

	for _, contractVersion := range sortedCompatibleContractVersions {
		contractGroupVersion := fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, contractVersion)

		// If there is no label, return early without changing the reference.
		supportedVersions, ok := labels[contractGroupVersion]
		if !ok || supportedVersions == "" {
			continue
		}

		// Pick the latest version in the slice and validate it.
		kubeVersions := util.KubeAwareAPIVersions(strings.Split(supportedVersions, "_"))
		sort.Sort(kubeVersions)
		return contractVersion, kubeVersions[len(kubeVersions)-1], nil
	}

	return "", "", errors.Errorf("cannot find any versions matching contract versions %q for CRD %v as contract version label(s) are either missing or empty (see https://cluster-api.sigs.k8s.io/developer/providers/contracts.html#api-version-labels)", sortedCompatibleContractVersions, metadata.GetName())
}

// GetContractVersionForVersion gets the contract version for a specific apiVersion.
func GetContractVersionForVersion(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, version string) (string, error) {
	crdMetadata, err := util.GetGVKMetadata(ctx, c, gvk)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get contract version")
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

	return "", errors.Errorf("cannot find any contract version matching version %q for CRD %v", version, crdMetadata.GetName())
}
