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

package profiles

import (
	"fmt"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/kubeadm"
)

// NewUbuntuAzureCluster creates a basic Digitalocean cluster profile, to bootstrap Kubernetes.
func NewUbuntuAzureCluster(name string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:     name,
		Cloud:    cluster.CloudAzure,
		Location: "eastus",
		SSH: &cluster.SSH{
			PublicKeyPath: "~/.ssh/id_rsa.pub",
			User:          "kubicorn",
		},
		Network: &cluster.Network{
			CIDR: "10.0.0.0/16",
		},
		KubernetesAPI: &cluster.KubernetesAPI{
			Port: "443",
		},
		Values: &cluster.Values{
			ItemMap: map[string]string{
				"INJECTEDTOKEN": kubeadm.GetRandomToken(),
			},
		},
		ServerPools: []*cluster.ServerPool{
			{
				Type:             cluster.ServerPoolTypeMaster,
				Name:             fmt.Sprintf("%s-master", name),
				MaxCount:         1,
				Image:            "UbuntuServer 16.04.0-LTS",
				Size:             "Standard_D4",
				BootstrapScripts: []string{"azure_k8s_ubuntu_16.04_master.sh"},
				Subnets: []*cluster.Subnet{
					{
						Name: fmt.Sprintf("%s-master-0", name),
						CIDR: "10.0.1.0/24",
						LoadBalancer: &cluster.LoadBalancer{
							InboundRules: []*cluster.InboundRule{
								{
									ListenPort: 22,
									TargetPort: 22,
								},
							},
						},
					},
				},
			},
			{
				Type:             cluster.ServerPoolTypeNode,
				Name:             fmt.Sprintf("%s-node", name),
				MaxCount:         1,
				Image:            "UbuntuServer 16.04.0-LTS",
				Size:             "Standard_D4",
				BootstrapScripts: []string{"azure_k8s_ubuntu_16.04_node.sh"},
				Subnets: []*cluster.Subnet{
					{
						Name: fmt.Sprintf("%s-node-0", name),
						CIDR: "10.0.100.0/24",
						LoadBalancer: &cluster.LoadBalancer{
							InboundRules: []*cluster.InboundRule{
								{
									ListenPort: 22,
									TargetPort: 22,
								},
							},
						},
					},
				},
			},
		},
	}
}
