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

// Package labels implements label utility functions.
package labels

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// IsTopologyOwned returns true if the object has the `topology.cluster.x-k8s.io/owned` label.
func IsTopologyOwned(o metav1.Object) bool {
	_, ok := o.GetLabels()[clusterv1.ClusterTopologyOwnedLabel]
	return ok
}

// HasWatchLabel returns true if the object has a label with the WatchLabel key matching the given value.
func HasWatchLabel(o metav1.Object, labelValue string) bool {
	val, ok := o.GetLabels()[clusterv1.WatchLabel]
	if !ok {
		return false
	}
	return val == labelValue
}
