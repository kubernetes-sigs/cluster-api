/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"

// Conditions and condition Reasons for the MachinePool object.

const (
	// ReplicasReadyV1Beta1Condition reports an aggregate of current status of the replicas controlled by the MachinePool.
	ReplicasReadyV1Beta1Condition clusterv1.ConditionType = "ReplicasReady"

	// WaitingForReplicasReadyV1Beta1Reason (Severity=Info) documents a machinepool waiting for the required replicas
	// to be ready.
	WaitingForReplicasReadyV1Beta1Reason = "WaitingForReplicasReady"
)
