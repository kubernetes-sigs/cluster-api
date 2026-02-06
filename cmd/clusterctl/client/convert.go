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

package client

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/convert"
)

var (
	// sourceGroupVersions defines the source GroupVersions that should be converted.
	sourceGroupVersions = []schema.GroupVersion{
		clusterv1.GroupVersion,
	}

	// knownAPIGroups defines all known API groups for resource classification.
	knownAPIGroups = []string{
		clusterv1.GroupVersion.Group,
	}
)

// ConvertOptions carries the options supported by Convert.
type ConvertOptions struct {
	// Input is the YAML content to convert.
	Input []byte

	// ToVersion is the target API version to convert to (e.g., "v1beta2").
	ToVersion string
}

// ConvertResult contains the result of a conversion operation.
type ConvertResult struct {
	// Output is the converted YAML content.
	Output []byte

	// Messages contains informational messages from the conversion.
	Messages []string
}

// Convert converts CAPI core resources between API versions.
func (c *clusterctlClient) Convert(_ context.Context, options ConvertOptions) (ConvertResult, error) {
	converter := convert.NewConverter(
		clusterv1.GroupVersion.Group, // targetAPIGroup: "cluster.x-k8s.io"
		clusterv1.GroupVersion,       // targetGV: schema.GroupVersion{Group: "cluster.x-k8s.io", Version: "v1beta2"}
		sourceGroupVersions,          // sourceGroupVersions
		knownAPIGroups,               // knownAPIGroups
	)

	output, msgs, err := converter.Convert(options.Input, options.ToVersion)
	if err != nil {
		return ConvertResult{}, err
	}

	return ConvertResult{
		Output:   output,
		Messages: msgs,
	}, nil
}
