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

// Package util implements etcd utility functions.
package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

// MemberForName returns the etcd member with the matching name.
func MemberForName(members []*etcd.Member, name string) *etcd.Member {
	for _, m := range members {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// MemberNames returns a list of all the etcd member names.
// Note: this function is specificially designed for MemberEqual and setting condition messages.
func MemberNames(members []*etcd.Member) []string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		// When adding a member the name may not yet be set.
		if m.Name == "" {
			names = append(names, fmt.Sprintf("name not set yet for member with id %d", m.ID))
			continue
		}
		names = append(names, m.Name)
	}
	return names
}

// MemberEqual returns true if the lists of members match.
//
// This function only checks that set of names of each member
// within the lists is the same.
func MemberEqual(members1, members2 []*etcd.Member) bool {
	names1 := sets.Set[string]{}.Insert(MemberNames(members1)...)
	names2 := sets.Set[string]{}.Insert(MemberNames(members2)...)
	return names1.Equal(names2)
}
