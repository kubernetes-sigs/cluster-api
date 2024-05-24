/*
Copyright 2023 The Kubernetes Authors.

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

// defines annotations to be applied to in memory etcd pods in order to track etcd cluster
// info belonging to the etcd member each pod represent.
const (
	// EtcdClusterIDAnnotationName defines the name of the annotation applied to in memory etcd
	// pods to track the cluster ID of the etcd member each pod represent.
	EtcdClusterIDAnnotationName = "etcd.inmemory.infrastructure.cluster.x-k8s.io/cluster-id"

	// EtcdMemberIDAnnotationName defines the name of the annotation applied to in memory etcd
	// pods to track the member ID of the etcd member each pod represent.
	EtcdMemberIDAnnotationName = "etcd.inmemory.infrastructure.cluster.x-k8s.io/member-id"

	// EtcdLeaderFromAnnotationName defines the name of the annotation applied to in memory etcd
	// pods to track leadership status of the etcd member each pod represent.
	// Note: We are tracking the time from an etcd member is leader; if more than one pod has this
	// annotation, the last etcd member that became leader is the current leader.
	// By using this mechanism leadership can be forwarded to another pod with an atomic operation
	// (add/update of the annotation to the pod/etcd member we are forwarding leadership to).
	EtcdLeaderFromAnnotationName = "etcd.inmemory.infrastructure.cluster.x-k8s.io/leader-from"

	// EtcdMemberRemoved is added to etcd pods which have been removed from the etcd cluster.
	EtcdMemberRemoved = "etcd.inmemory.infrastructure.cluster.x-k8s.io/member-removed"
)
