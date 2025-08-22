/*
Copyright 2022 The Kubernetes Authors.

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

import "sync"

// BootstrapContract encodes information about the Cluster API contract for bootstrap objects.
type BootstrapContract struct{}

var bootstrap *BootstrapContract
var onceBootstrap sync.Once

// Bootstrap provide access to the information about the Cluster API contract for bootstrap objects.
func Bootstrap() *BootstrapContract {
	onceBootstrap.Do(func() {
		bootstrap = &BootstrapContract{}
	})
	return bootstrap
}

// DataSecretCreated returns if the data secret has been created.
func (b *BootstrapContract) DataSecretCreated(contractVersion string) *Bool {
	if contractVersion == "v1beta1" {
		return &Bool{
			path: []string{"status", "ready"},
		}
	}

	return &Bool{
		path: []string{"status", "initialization", "dataSecretCreated"},
	}
}

// ReadyConditionType returns the type of the ready condition.
func (b *BootstrapContract) ReadyConditionType() string {
	return "Ready" //nolint:goconst // Not making this a constant for now
}

// DataSecretName provide access to status.dataSecretName field in a bootstrap object.
func (b *BootstrapContract) DataSecretName() *String {
	return &String{
		path: []string{"status", "dataSecretName"},
	}
}

// FailureReason provides access to the status.failureReason field in an bootstrap object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (b *BootstrapContract) FailureReason() *String {
	return &String{
		path: []string{"status", "failureReason"},
	}
}

// FailureMessage provides access to the status.failureMessage field in an bootstrap object. Note that this field is optional.
//
// Deprecated: This function is deprecated and is going to be removed. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
func (b *BootstrapContract) FailureMessage() *String {
	return &String{
		path: []string{"status", "failureMessage"},
	}
}
