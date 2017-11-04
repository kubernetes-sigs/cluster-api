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

package resources

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

var _ cloud.Resource = &ResourceGroup{}

type ResourceGroup struct {
	Shared
	Location string
}

// Actual returns the actual resource group in Azure if it exists.
func (r *ResourceGroup) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("resourcegroup.Actual")
	newResource := &ResourceGroup{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: immutable.GroupIdentifier,
		},
		Location: r.Location,
	}

	if r.Identifier != "" {
		group, err := Sdk.ResourceGroup.Get(immutable.Name)
		if err != nil {
			return nil, nil, err
		}
		newResource.Location = *group.Location
		newResource.Name = *group.Name
		newResource.Identifier = *group.ID
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

// Expected will return the expected resource group as it would be defined in Azure
func (r *ResourceGroup) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("resourcegroup.Expected")
	newResource := &ResourceGroup{
		Shared: Shared{
			Name:       immutable.Name,
			Tags:       r.Tags,
			Identifier: immutable.GroupIdentifier,
		},
		Location: immutable.Location,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *ResourceGroup) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("resourcegroup.Apply")
	applyResource := expected.(*ResourceGroup)
	isEqual, err := compare.IsEqual(actual.(*ResourceGroup), expected.(*ResourceGroup))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	group, err := Sdk.ResourceGroup.CreateOrUpdate(immutable.Name, resources.Group{
		Location: &immutable.Location,
	})
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Created resource group [%s]", *group.Name)

	newResource := &ResourceGroup{
		Shared: Shared{
			Name: *group.Name,
		},
		Location: *group.Location,
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *ResourceGroup) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("resourcegroup.Delete")
	deleteResource := actual.(*ResourceGroup)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}

	autorestChan, errorChan := Sdk.ResourceGroup.Delete(immutable.ClusterName, make(chan struct{}))
	select {
	case <-autorestChan:
		logger.Info("Successfully deleted resource group [%s]", deleteResource.Identifier)
	case err := <-errorChan:
		return nil, nil, err
	}

	newResource := &ResourceGroup{
		Shared: Shared{
			Name: immutable.Name,
			Tags: r.Tags,
		},
		Location: immutable.Location,
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *ResourceGroup) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("resourcegroup.Render")
	resourceGroup := newResource.(*ResourceGroup)
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	newCluster.GroupIdentifier = resourceGroup.Identifier
	newCluster.Location = resourceGroup.Location
	newCluster.Name = resourceGroup.Name
	return newCluster
}
