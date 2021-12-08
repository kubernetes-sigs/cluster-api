/*
Copyright 2021 The Kubernetes Authors.

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

// Package index provides indexes for the api.
package index

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/feature"
)

// AddDefaultIndexes registers the default list of indexes.
func AddDefaultIndexes(ctx context.Context, mgr ctrl.Manager) error {
	if err := ByMachineNode(ctx, mgr); err != nil {
		return err
	}

	if err := ByMachineProviderID(ctx, mgr); err != nil {
		return err
	}

	if feature.Gates.Enabled(feature.ClusterTopology) {
		if err := ByClusterClassName(ctx, mgr); err != nil {
			return err
		}
	}

	return nil
}
