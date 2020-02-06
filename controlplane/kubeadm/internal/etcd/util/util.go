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

package util

import (
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

// MemberStarted checks whether a member has started.
func MemberStarted(member *etcd.Member) bool {
	return len(member.Name) > 0
}

// MemberForName returns the etcd member with the matching name.
func MemberForName(members []*etcd.Member, name string) *etcd.Member {
	for _, m := range members {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// MemberIDSet returns a set of member IDs.
func MemberIDSet(members []*etcd.Member) UInt64 {
	set := UInt64{}
	for _, m := range members {
		set.Insert(m.ID)
	}
	return set
}
