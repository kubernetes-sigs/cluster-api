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

package test

import (
	clusterv1old "sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// PreviousCAPIContractNotSupported define the previous Cluster API contract, not supported by this release of clusterctl.
var PreviousCAPIContractNotSupported = clusterv1old.GroupVersion.Version

// CurrentCAPIContract define the current Cluster API contract.
var CurrentCAPIContract = clusterv1.GroupVersion.Version

// NextCAPIContractNotSupported define the next Cluster API contract, not supported by this release of clusterctl.
const NextCAPIContractNotSupported = "v99"
