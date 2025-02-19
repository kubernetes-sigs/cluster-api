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
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// IsTopologyOwned returns true if the object has the `topology.cluster.x-k8s.io/owned` label.
func IsTopologyOwned(o metav1.Object) bool {
	_, ok := o.GetLabels()[clusterv1.ClusterTopologyOwnedLabel]
	return ok
}

// IsMachinePoolOwned returns true if the object has the `cluster.x-k8s.io/pool-name` label or is owned by a MachinePool.
func IsMachinePoolOwned(o metav1.Object) bool {
	if _, ok := o.GetLabels()[clusterv1.MachinePoolNameLabel]; ok {
		return true
	}

	for _, owner := range o.GetOwnerReferences() {
		if owner.Kind == "MachinePool" {
			return true
		}
	}

	return false
}

// HasWatchLabel returns true if the object has a label with the WatchLabel key matching the given value.
func HasWatchLabel(o metav1.Object, labelValue string) bool {
	val, ok := o.GetLabels()[clusterv1.WatchLabel]
	if !ok {
		return false
	}
	return val == labelValue
}

// GetManagedLabels returns the set of labels managed by CAPI, with an optional list of regex patterns for user-specified labels.
func GetManagedLabels(labels map[string]string, additionalSyncMachineLabels ...*regexp.Regexp) map[string]string {
	managedLabels := make(map[string]string)
	for key, value := range labels {
		// Always sync the default set of labels.
		dnsSubdomainOrName := strings.Split(key, "/")[0]
		if dnsSubdomainOrName == clusterv1.NodeRoleLabelPrefix {
			managedLabels[key] = value
			continue
		}
		if dnsSubdomainOrName == clusterv1.NodeRestrictionLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+clusterv1.NodeRestrictionLabelDomain) {
			managedLabels[key] = value
			continue
		}
		if dnsSubdomainOrName == clusterv1.ManagedNodeLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+clusterv1.ManagedNodeLabelDomain) {
			managedLabels[key] = value
			continue
		}

		// Sync if the labels matches at least one user provided regex.
		for _, regex := range additionalSyncMachineLabels {
			if regex.MatchString(key) {
				managedLabels[key] = value
				break
			}
		}
	}
	return managedLabels
}
