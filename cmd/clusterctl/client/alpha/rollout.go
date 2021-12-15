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

package alpha

import (
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// MachineDeployment is a resource type.
const MachineDeployment = "machinedeployment"

var validResourceTypes = []string{MachineDeployment}

// Rollout defines the behavior of a rollout implementation.
type Rollout interface {
	ObjectRestarter(cluster.Proxy, corev1.ObjectReference) error
	ObjectPauser(cluster.Proxy, corev1.ObjectReference) error
	ObjectResumer(cluster.Proxy, corev1.ObjectReference) error
	ObjectRollbacker(cluster.Proxy, corev1.ObjectReference, int64) error
}

var _ Rollout = &rollout{}

type rollout struct{}

func newRolloutClient() Rollout {
	return &rollout{}
}
