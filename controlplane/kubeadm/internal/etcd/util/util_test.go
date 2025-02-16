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
	"reflect"
	"testing"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

func TestMemberEqual(t *testing.T) {
	tests := []struct {
		name     string
		members1 []*etcd.Member
		members2 []*etcd.Member
		want     bool
	}{
		{
			name:     "Not matching member lists",
			members1: []*etcd.Member{{ID: 1, Name: "foo"}},
			members2: []*etcd.Member{{ID: 2, Name: "bar"}},
			want:     false,
		},
		{
			name:     "Matching member lists",
			members1: []*etcd.Member{{ID: 1, Name: "foo"}},
			members2: []*etcd.Member{{ID: 1, Name: "foo"}},
			want:     true,
		},
		{
			name:     "Matching member lists having multiple entries",
			members1: []*etcd.Member{{ID: 1, Name: "foo"}, {ID: 2, Name: "bar"}, {ID: 3, Name: "foobar"}},
			members2: []*etcd.Member{{ID: 1, Name: "foo"}, {ID: 2, Name: "bar"}, {ID: 3, Name: "foobar"}},
			want:     true,
		},
		{
			name:     "Matching member lists having multiple entries which are not ordered",
			members1: []*etcd.Member{{ID: 1, Name: "foo"}, {ID: 2, Name: "bar"}, {ID: 3, Name: "foobar"}},
			members2: []*etcd.Member{{ID: 2, Name: "bar"}, {ID: 1, Name: "foo"}, {ID: 3, Name: "foobar"}},
			want:     true,
		},
		{
			name:     "Matching member lists including a empty name",
			members1: []*etcd.Member{{ID: 1, Name: "foo"}, {ID: 2, Name: "bar"}, {ID: 3, Name: "foobar"}, {ID: 4, Name: ""}},
			members2: []*etcd.Member{{ID: 2, Name: "bar"}, {ID: 1, Name: "foo"}, {ID: 3, Name: "foobar"}, {ID: 4, Name: ""}},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemberEqual(tt.members1, tt.members2); got != tt.want {
				t.Errorf("MemberEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemberNames(t *testing.T) {
	tests := []struct {
		name    string
		members []*etcd.Member
		want    []string
	}{
		{
			name:    "Empty members list",
			members: []*etcd.Member{},
			want:    []string{},
		},
		{
			name:    "Member without name",
			members: []*etcd.Member{{ID: 0}},
			want:    []string{"name not set yet for member with id 0"},
		},
		{
			name:    "Member with name",
			members: []*etcd.Member{{ID: 1, Name: "foo"}},
			want:    []string{"foo"},
		},
		{
			name:    "Multiple members",
			members: []*etcd.Member{{ID: 0}, {ID: 2, Name: "bar"}, {ID: 1, Name: "foo"}, {ID: 3, Name: "foobar"}},
			want:    []string{"name not set yet for member with id 0", "bar", "foo", "foobar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemberNames(tt.members); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MemberNames() = %v, want %v", got, tt.want)
			}
		})
	}
}
