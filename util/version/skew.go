/*
Copyright 2026 The Kubernetes Authors.

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

package version

import "github.com/blang/semver/v4"

// MaxWorkerMinorVersionSkew is the number of minor versions a worker (kubelet) is allowed to lag
// behind the control plane (kube-apiserver).
// See https://kubernetes.io/releases/version-skew-policy/#kubelet
const MaxWorkerMinorVersionSkew = uint64(3)

// WorkerVersionSkewSupported returns true if worker conforms to the Kubernetes version skew policy
// relative to controlPlane: it must be of the same major version, must not be on a newer minor
// version than the control plane, and must not be more than MaxWorkerMinorVersionSkew minor
// versions older.
func WorkerVersionSkewSupported(controlPlane, worker semver.Version) bool {
	if worker.Major != controlPlane.Major {
		return false
	}
	if worker.Minor > controlPlane.Minor {
		return false
	}
	// Note: the check above guarantees worker.Minor <= controlPlane.Minor, which is what keeps this
	// unsigned subtraction from underflowing when the control plane minor version is lower than the
	// tolerated skew. Do not reorder or drop it.
	return controlPlane.Minor-worker.Minor <= MaxWorkerMinorVersionSkew
}

// KubeadmJoinVersionSkewSupported returns true if a worker at version worker can join a control
// plane at version controlPlane using kubeadm, which only supports joining with the same major and
// minor version as the control plane.
// See https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#kubeadm-s-skew-against-kubeadm
func KubeadmJoinVersionSkewSupported(controlPlane, worker semver.Version) bool {
	return worker.Major == controlPlane.Major && worker.Minor == controlPlane.Minor
}
