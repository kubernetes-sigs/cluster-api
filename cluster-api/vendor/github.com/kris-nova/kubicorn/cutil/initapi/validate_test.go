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
	"testing"

	"github.com/kris-nova/kubicorn/apis/cluster"
)

func TestValidateAtLeastOneServerPoolHappy(t *testing.T) {
	c := &cluster.Cluster{
		Name: "c",
		ServerPools: []*cluster.ServerPool{
			{},
		},
	}
	err := validateAtLeastOneServerPool(c)
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"should be: nil\n"+
			"got:       %v\n", err)
	}
}

func TestValidateAtLeastOneServerPoolSad(t *testing.T) {
	c := cluster.NewCluster("c")
	expected := "cluster c must have at least one server pool"
	err := validateAtLeastOneServerPool(c)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if err.Error() != expected {
		t.Fatalf("error message incorrect\n"+
			"should be: %v\n"+
			"got:       %v\n", expected, err.Error())
	}
}

func TestValidateServerPoolMaxCountGreaterThan1Happy(t *testing.T) {
	c := &cluster.Cluster{
		Name: "c",
		ServerPools: []*cluster.ServerPool{
			{
				Name:     "p",
				MaxCount: 1,
			},
		},
	}
	err := validateServerPoolMaxCountGreaterThan1(c)
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"should be: nil\n"+
			"got:       %v\n", err)
	}
}

func TestValidateServerPoolMaxCountGreaterThan1Sad(t *testing.T) {
	c := &cluster.Cluster{
		Name: "c",
		ServerPools: []*cluster.ServerPool{
			{
				Name:     "p",
				MaxCount: 0,
			},
		},
	}
	expected := "server pool p in cluster c must have a maximum count greater than 0"
	err := validateServerPoolMaxCountGreaterThan1(c)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if err.Error() != expected {
		t.Fatalf("error message incorrect\n"+
			"should be: %v\n"+
			"got:       %v\n", expected, err.Error())
	}
}

func TestValidateSpotPriceOnlyForAwsClusterHappy(t *testing.T) {
	c := &cluster.Cluster{
		Name:  "c",
		Cloud: "amazon",
		ServerPools: []*cluster.ServerPool{
			{
				Name: "p",
				AwsConfiguration: &cluster.AwsConfiguration{
					SpotPrice: "1",
				},
			},
		},
	}
	err := validateSpotPriceOnlyForAwsCluster(c)
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"should be: nil\n"+
			"got:       %v\n", err)
	}
}

func TestValidateSpotPriceOnlyForAwsClusterSad(t *testing.T) {
	c := &cluster.Cluster{
		Name:  "c",
		Cloud: "azure",
		ServerPools: []*cluster.ServerPool{
			{
				Name: "p",
				AwsConfiguration: &cluster.AwsConfiguration{
					SpotPrice: "1",
				},
			},
		},
	}
	expected := "Spot price provided for server pool p can only be used with AWS"
	err := validateSpotPriceOnlyForAwsCluster(c)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if err.Error() != expected {
		t.Fatalf("error message incorrect\n"+
			"should be: %v\n"+
			"got:       %v\n", expected, err.Error())
	}
}
