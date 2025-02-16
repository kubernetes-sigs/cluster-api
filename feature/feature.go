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

// Package feature implements feature functionality.
package feature

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
	// Every feature gate should add method here following this template:
	//
	// // owner: @username
	// // alpha: v1.X
	// MyFeature featuregate.Feature = "MyFeature".

	// MachinePool is a feature gate for MachinePool functionality.
	//
	// alpha: v0.3
	// beta: v1.7
	MachinePool featuregate.Feature = "MachinePool"

	// ClusterResourceSet is a feature gate for the ClusterResourceSet functionality.
	//
	// alpha: v0.3
	// beta: v0.4
	// GA: v1.10
	//
	// Deprecated: ClusterResourceSet feature is now GA and the corresponding feature flag will be removed in 1.12 release.
	ClusterResourceSet featuregate.Feature = "ClusterResourceSet"

	// ClusterTopology is a feature gate for the ClusterClass and managed topologies functionality.
	//
	// alpha: v0.4
	ClusterTopology featuregate.Feature = "ClusterTopology"

	// RuntimeSDK is a feature gate for the Runtime hooks and extensions functionality.
	//
	// alpha: v1.2
	RuntimeSDK featuregate.Feature = "RuntimeSDK"

	// KubeadmBootstrapFormatIgnition is a feature gate for the Ignition bootstrap format
	// functionality.
	//
	// alpha: v1.1
	KubeadmBootstrapFormatIgnition featuregate.Feature = "KubeadmBootstrapFormatIgnition"

	// MachineSetPreflightChecks is a feature gate for the MachineSet preflight checks functionality.
	//
	// alpha: v1.5
	// beta: v1.9
	MachineSetPreflightChecks featuregate.Feature = "MachineSetPreflightChecks"

	// MachineWaitForVolumeDetachConsiderVolumeAttachments is a feature gate that controls if the Machine controller
	// also considers VolumeAttachments in addition to Nodes.status.volumesAttached when waiting for volumes to be detached.
	//
	// beta: v1.9
	MachineWaitForVolumeDetachConsiderVolumeAttachments featuregate.Feature = "MachineWaitForVolumeDetachConsiderVolumeAttachments"

	// PriorityQueue is a feature gate that controls if the controller uses the controller-runtime PriorityQueue
	// instead of the default queue implementation.
	//
	// alpha: v1.10
	PriorityQueue featuregate.Feature = "PriorityQueue"
)

func init() {
	runtime.Must(MutableGates.Add(defaultClusterAPIFeatureGates))
}

// defaultClusterAPIFeatureGates consists of all known cluster-api-specific feature keys.
// To add a new feature, define a key for it above and add it here.
var defaultClusterAPIFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	// Every feature should be initiated here:
	ClusterResourceSet:        {Default: true, PreRelease: featuregate.GA},
	MachinePool:               {Default: true, PreRelease: featuregate.Beta},
	MachineSetPreflightChecks: {Default: true, PreRelease: featuregate.Beta},
	MachineWaitForVolumeDetachConsiderVolumeAttachments: {Default: true, PreRelease: featuregate.Beta},
	PriorityQueue:                  {Default: false, PreRelease: featuregate.Alpha},
	ClusterTopology:                {Default: false, PreRelease: featuregate.Alpha},
	KubeadmBootstrapFormatIgnition: {Default: false, PreRelease: featuregate.Alpha},
	RuntimeSDK:                     {Default: false, PreRelease: featuregate.Alpha},
}
