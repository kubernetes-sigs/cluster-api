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

package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	// PreflightCheckCondition documents a Provider that has not passed preflight checks.
	PreflightCheckCondition clusterv1.ConditionType = "PreflightCheckPassed"

	// MoreThanOneProviderInstanceExistsReason (Severity=Info) documents that more than one instance of provider
	// exists in the cluster.
	MoreThanOneProviderInstanceExistsReason = "MoreThanOneExists"

	// IncorrectVersionFormatReason documents that the provider version is in the incorrect format.
	IncorrectVersionFormatReason = "IncorrectVersionFormat"

	// FetchConfigValidationError documents that the FetchConfig is configured incorrectly.
	FetchConfigValidationErrorReason = "FetchConfigValidationError"

	// UnknownProviderReason documents that the provider name is not the name of a know provider.
	UnknownProviderReason = "UnknownProvider"

	// CAPIVersionIncompatibility documents that the provider version is incompatible with operator.
	CAPIVersionIncompatibilityReason = "CAPIVersionIncompatibility"

	// ComponentsFetchErrorReason documents that an error occurred fetching the componets.
	ComponentsFetchErrorReason = "ComponentsFetchError"

	// OldComponentsDeletionErrorReason documents that an error occurred deleting the old components prior to upgrading.
	OldComponentsDeletionErrorReason = "OldComponentsDeletionError"

	// WaitingForCoreProviderReadyReason documents that the provider is waiting for the core provider to be ready.
	WaitingForCoreProviderReadyReason = "WaitingForCoreProviderReady"
)

const (
	// CertManagerReadyCondition documents a Provider that does not have dependent CertManager Ready.
	CertManagerReadyCondition clusterv1.ConditionType = "CertManagerReady"
)

const (
	// ProviderInstalledCondition documents a Provider that has been installed.
	ProviderInstalledCondition clusterv1.ConditionType = "ProviderInstalled"
)
