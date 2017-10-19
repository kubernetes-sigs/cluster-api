// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package initapi

import (
	"fmt"

	"github.com/kris-nova/kubicorn/apis/cluster"
)

func validateAtLeastOneServerPool(initCluster *cluster.Cluster) error {
	if len(initCluster.ServerPools) < 1 {
		return fmt.Errorf("cluster %v must have at least one server pool", initCluster.Name)
	}
	return nil
}

func validateServerPoolMaxCountGreaterThan1(initCluster *cluster.Cluster) error {
	for _, p := range initCluster.ServerPools {
		if p.MaxCount < 1 {
			return fmt.Errorf("server pool %v in cluster %v must have a maximum count greater than 0", p.Name, initCluster.Name)
		}
	}
	return nil
}

func validateSpotPriceOnlyForAwsCluster(initCluster *cluster.Cluster) error {
	for _, p := range initCluster.ServerPools {
		if p.AwsConfiguration != nil && p.AwsConfiguration.SpotPrice != "" && initCluster.Cloud != cluster.CloudAmazon {
			return fmt.Errorf("Spot price provided for server pool %v can only be used with AWS", p.Name)
		}
	}
	return nil
}
