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

package azure

import (
	"fmt"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/kubeadm"
)

// NewUbuntuCluster creates a basic Azure cluster profile, to bootstrap Kubernetes.
func NewUbuntuCluster(name string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:     name,
		Cloud:    cluster.CloudAzure,
		Location: "eastus",
		SSH: &cluster.SSH{
			PublicKeyPath: "~/.ssh/id_rsa.pub",
			User:          "root",
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
				Image:            "UbuntuServer",
				Size:             "Standard_DS3_v2 ",
				BootstrapScripts: []string{},
				Firewalls: []*cluster.Firewall{
					{
						Name: fmt.Sprintf("%s-master", name),
						IngressRules: []*cluster.IngressRule{
							{
								IngressToPort:   "22",
								IngressSource:   "0.0.0.0/0",
								IngressProtocol: "tcp",
							},
							{
								IngressToPort:   "443",
								IngressSource:   "0.0.0.0/0",
								IngressProtocol: "tcp",
							},
							{
								IngressToPort:   "1194",
								IngressSource:   "0.0.0.0/0",
								IngressProtocol: "udp",
							},
						},
						EgressRules: []*cluster.EgressRule{
							{
								EgressToPort:      "all", // By default all egress from VM
								EgressDestination: "0.0.0.0/0",
								EgressProtocol:    "tcp",
							},
							{
								EgressToPort:      "all", // By default all egress from VM
								EgressDestination: "0.0.0.0/0",
								EgressProtocol:    "udp",
							},
						},
					},
				},
			},
			{
				Type:             cluster.ServerPoolTypeNode,
				Name:             fmt.Sprintf("%s-node", name),
				MaxCount:         1,
				Image:            "UbuntuServer",
				Size:             "Standard_DS3_v2 ",
				BootstrapScripts: []string{},
				Firewalls: []*cluster.Firewall{
					{
						Name: fmt.Sprintf("%s-node", name),
						IngressRules: []*cluster.IngressRule{
							{
								IngressToPort:   "22",
								IngressSource:   "0.0.0.0/0",
								IngressProtocol: "tcp",
							},
							{
								IngressToPort:   "1194",
								IngressSource:   "0.0.0.0/0",
								IngressProtocol: "udp",
							},
						},
						EgressRules: []*cluster.EgressRule{
							{
								EgressToPort:      "all", // By default all egress from VM
								EgressDestination: "0.0.0.0/0",
								EgressProtocol:    "tcp",
							},
							{
								EgressToPort:      "all", // By default all egress from VM
								EgressDestination: "0.0.0.0/0",
								EgressProtocol:    "udp",
							},
						},
					},
				},
			},
		},
	}
}
