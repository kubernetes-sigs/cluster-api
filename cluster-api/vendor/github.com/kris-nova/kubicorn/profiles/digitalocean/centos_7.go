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

package digitalocean

import (
	"fmt"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/kubeadm"
)

// NewCentosCluster creates a basic CentOS DigitalOcean cluster.
func NewCentosCluster(name string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:     name,
		Cloud:    cluster.CloudDigitalOcean,
		Location: "sfo2",
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
				Type:     cluster.ServerPoolTypeMaster,
				Name:     fmt.Sprintf("%s-master", name),
				MaxCount: 1,
				Image:    "centos-7-x64",
				Size:     "2gb",
				BootstrapScripts: []string{
					"bootstrap/vpn/openvpnMaster-centos.sh",
					"bootstrap/digitalocean_k8s_centos_7_master.sh",
				},
			},
			{
				Type:     cluster.ServerPoolTypeNode,
				Name:     fmt.Sprintf("%s-node", name),
				MaxCount: 1,
				Image:    "centos-7-x64",
				Size:     "1gb",
				BootstrapScripts: []string{
					"bootstrap/vpn/openvpnNode-centos.sh",
					"bootstrap/digitalocean_k8s_centos_7_node.sh",
				},
			},
		},
	}
}
