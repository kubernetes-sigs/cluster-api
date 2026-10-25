/*
Copyright 2024 The Kubernetes Authors.

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

package v1beta1

// These reasons are intended to be used with failed Ready conditions on an [IPAddressClaim].
const (
	// AllocationFailedReason is the reason used when allocating an IP address for a claim fails. More details should be
	// provided in the condition's message.
	// When the IP pool is full, [PoolExhaustedReason] should be used for better visibility instead.
	AllocationFailedReason = "AllocationFailed"

	// PoolNotReadyReason is the reason used when the referenced IP pool is not ready.
	PoolNotReadyReason = "PoolNotReady"

	// PoolExhaustedReason is the reason used when an IP pool referenced by an [IPAddressClaim] is full and no address
	// can be allocated for the claim.
	PoolExhaustedReason = "PoolExhausted"
)
