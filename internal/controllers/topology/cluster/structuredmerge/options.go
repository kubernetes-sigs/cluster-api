/*
Copyright 2022 The Kubernetes Authors.

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

package structuredmerge

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
)

var (
	// defaultAllowedPaths are the allowed paths for all objects except Clusters.
	defaultAllowedPaths = []contract.Path{
		// apiVersion, kind, name and namespace are required field for a server side apply intent.
		{"apiVersion"},
		{"kind"},
		{"metadata", "name"},
		{"metadata", "namespace"},
		// uid is optional for a server side apply intent but sets the expectation of an object getting created or a specific one updated.
		{"metadata", "uid"},
		// the topology controller controls/has an opinion for labels, annotation, ownerReferences and spec only.
		{"metadata", "labels"},
		{"metadata", "annotations"},
		{"metadata", "ownerReferences"},
		{"spec"},
	}

	// allowedPathsCluster are the allowed paths specific for Clusters.
	// The cluster object is not created by the topology controller and already contains fields which are
	// not supposed for the topology controller to have an opinion on / take (co-)ownership of.
	// Because of that the allowedPaths are different to other objects.
	// NOTE: This is mostly the same as defaultAllowedPaths but having more restrictions.
	allowedPathsCluster = []contract.Path{
		// apiVersion, kind, name and namespace are required field for a server side apply intent.
		{"apiVersion"},
		{"kind"},
		{"metadata", "name"},
		{"metadata", "namespace"},
		// uid is optional for a server side apply intent but sets the expectation of an object getting created or a specific one updated.
		{"metadata", "uid"},
		// the topology controller controls/has an opinion for the labels ClusterNameLabel
		// and ClusterTopologyOwnedLabel as well as infrastructureRef and controlPlaneRef in spec.
		{"metadata", "labels", clusterv1.ClusterNameLabel},
		{"metadata", "labels", clusterv1.ClusterTopologyOwnedLabel},
		{"spec", "infrastructureRef"},
		{"spec", "controlPlaneRef"},
	}
)

// HelperOption is some configuration that modifies options for Helper.
type HelperOption interface {
	// ApplyToHelper applies this configuration to the given helper options.
	ApplyToHelper(*HelperOptions)
}

// HelperOptions contains options for Helper.
type HelperOptions struct {
	ssa.FilterObjectInput
}

// newHelperOptions returns initialized HelperOptions.
func newHelperOptions(target client.Object, opts ...HelperOption) *HelperOptions {
	helperOptions := &HelperOptions{
		FilterObjectInput: ssa.FilterObjectInput{
			AllowedPaths: defaultAllowedPaths,
			IgnorePaths:  []contract.Path{},
		},
	}
	// Overwrite the allowedPaths for Cluster objects to prevent the topology controller
	// to take ownership of fields it is not supposed to.
	if _, ok := target.(*clusterv1.Cluster); ok {
		helperOptions.AllowedPaths = allowedPathsCluster
	}
	helperOptions = helperOptions.ApplyOptions(opts)
	return helperOptions
}

// ApplyOptions applies the given patch options on these options,
// and then returns itself (for convenient chaining).
func (o *HelperOptions) ApplyOptions(opts []HelperOption) *HelperOptions {
	for _, opt := range opts {
		opt.ApplyToHelper(o)
	}
	return o
}

// IgnorePaths instruct the Helper to ignore given paths when computing a patch.
// NOTE: ignorePaths are used to filter out fields nested inside allowedPaths, e.g.
// spec.ControlPlaneEndpoint.
type IgnorePaths []contract.Path

// ApplyToHelper applies this configuration to the given helper options.
func (i IgnorePaths) ApplyToHelper(opts *HelperOptions) {
	opts.IgnorePaths = i
}
