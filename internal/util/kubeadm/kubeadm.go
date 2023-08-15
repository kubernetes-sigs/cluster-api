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

// Package kubeadm contains utils related to kubeadm.
package kubeadm

import "github.com/blang/semver/v4"

const (
	// DefaultImageRepository is the new default Kubernetes image registry.
	DefaultImageRepository = "registry.k8s.io"
	// OldDefaultImageRepository is the old default Kubernetes image registry.
	OldDefaultImageRepository = "k8s.gcr.io"
)

var (
	// MinKubernetesVersionImageRegistryMigration is the first Kubernetes minor version which
	// has patch versions where the default image registry in kubeadm is registry.k8s.io instead of k8s.gcr.io.
	MinKubernetesVersionImageRegistryMigration = semver.MustParse("1.22.0")

	// NextKubernetesVersionImageRegistryMigration is the next minor version after
	// the default image registry in kubeadm changed to registry.k8s.io.
	NextKubernetesVersionImageRegistryMigration = semver.MustParse("1.26.0")
)

// GetDefaultRegistry returns the default registry of the given kubeadm version.
func GetDefaultRegistry(version semver.Version) string {
	// If version <= v1.22.16 return k8s.gcr.io
	if version.LTE(semver.MustParse("1.22.16")) {
		return OldDefaultImageRepository
	}

	// If v1.22.17 <= version < v1.23.0 return registry.k8s.io
	if version.GTE(semver.MustParse("1.22.17")) &&
		version.LT(semver.MustParse("1.23.0")) {
		return DefaultImageRepository
	}

	// If v1.23.0  <= version <= v1.23.14 return k8s.gcr.io
	if version.GTE(semver.MustParse("1.23.0")) &&
		version.LTE(semver.MustParse("1.23.14")) {
		return OldDefaultImageRepository
	}

	// If v1.23.15 <= version < v1.24.0 return registry.k8s.io
	if version.GTE(semver.MustParse("1.23.15")) &&
		version.LT(semver.MustParse("1.24.0")) {
		return DefaultImageRepository
	}

	// If v1.24.0  <= version <= v1.24.8 return k8s.gcr.io
	if version.GTE(semver.MustParse("1.24.0")) &&
		version.LTE(semver.MustParse("1.24.8")) {
		return OldDefaultImageRepository
	}

	// If v1.24.9  <= version return registry.k8s.io
	return DefaultImageRepository
}
