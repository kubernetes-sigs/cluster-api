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

// Package v1beta2 implements utils for metav1.Conditions that will be used starting with the v1beta2 API.
//
// Please note that in order to make this change while respecting API deprecation rules, it is required
// to go through a phased approach:
// - Phase 1. metav1.Conditions will be added into v1beta1 API types under the Status.V1Beta2.Conditions struct (clusterv1.Conditions will remain in Status.Conditions)
// - Phase 2. when introducing v1beta2 API types:
//   - clusterv1.Conditions will be moved from Status.Conditions to Status.Deprecated.V1Beta1.Conditions
//   - metav1.Conditions will be moved from Status.V1Beta2.Conditions to Status.Conditions
//
// - Phase 3. when removing v1beta1 API types, Status.Deprecated will be dropped.
//
// Please see the proposal https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
//
// In order to make this transition easier both for CAPI and other projects using this package,
// utils automatically adapt to handle objects at different stage of the transition.
package v1beta2
