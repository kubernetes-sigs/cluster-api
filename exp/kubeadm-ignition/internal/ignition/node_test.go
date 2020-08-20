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

package ignition

import (
	types "sigs.k8s.io/cluster-api/exp/kubeadm-ignition/types/v1beta1"
	"testing"

	"sigs.k8s.io/cluster-api/exp/kubeadm-ignition/api/v1alpha3"
)

func TestToUserData(t *testing.T) {
	factory := Factory{dataSource: &FakeBackend{}}
	for _, testcase := range testCases {
		if data, err := factory.GenerateUserData(&testcase.input); err != nil {
			t.Error(err)
		} else if string(data) != testcase.expected {
			t.Errorf("got \n %s \n expected\n %s\n", string(data), testcase.expected)
		}
	}
}

var testCases = []struct {
	expected string
	input    Node
}{
	{
		expected: `{"ignition":{"config":{},"security":{"tls":{}},"timeouts":{},"version":"2.2.0"},"networkd":{},"passwd":{},"storage":{"files":[{"filesystem":"root","overwrite":true,"path":"/etc/docker/daemon.json","contents":{"source":"data:,%7B%22bridge%22%3A%22none%22%2C%22log-driver%22%3A%20%22json-file%22%2C%22log-opts%22%3A%20%7B%22max-size%22%3A%20%2210m%22%2C%22max-file%22%3A%20%2210%22%7D%2C%22live-restore%22%3A%20true%2C%22max-concurrent-downloads%22%3A10%7D%0A","verification":{}},"mode":420}]},"systemd":{}}`,
		input: Node{
			Files: []v1alpha3.File{
				{
					Path:        "/etc/docker/daemon.json",
					Permissions: "0644",
					Content: `{"bridge":"none","log-driver": "json-file","log-opts": {"max-size": "10m","max-file": "10"},"live-restore": true,"max-concurrent-downloads":10}
`,
				},
			},
			Version: "v1.17.4",
		},
	},
	{
		expected: `{"ignition":{"config":{},"security":{"tls":{}},"timeouts":{},"version":"2.2.0"},"networkd":{},"passwd":{},"storage":{},"systemd":{"units":[{"contents":"[Unit]\nDescription=extract k8s files\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/tar xzvf /opt/kubernetes.tar.gz -C /\n\n[Install]\nWantedBy=multi-user.target\n","enabled":true,"name":"extractk8s.service"}]}}`,
		input: Node{
			Services: []types.ServiceUnit{
				{
					Content: "[Unit]\nDescription=extract k8s files\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/tar xzvf /opt/kubernetes.tar.gz -C /\n\n[Install]\nWantedBy=multi-user.target\n",
					Enabled: true,
					Name:    "extractk8s.service",
				},
			},
			Version: "v1.16.8",
		},
	},
	{
		expected: `{"ignition":{"config":{},"security":{"tls":{}},"timeouts":{},"version":"2.2.0"},"networkd":{},"passwd":{},"storage":{},"systemd":{"units":[{"contents":"[Unit]\nDescription=extract k8s files\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/tar xzvf /opt/kubernetes.tar.gz -C /\n\n[Install]\nWantedBy=multi-user.target\n","dropins":[{"contents":"[Service]\nLimitMEMLOCK=infinity\n","name":"30-increase-ulimit.conf"}],"enabled":true,"name":"extractk8s.service"}]}}`,
		input: Node{
			Services: []types.ServiceUnit{
				{
					Content: "[Unit]\nDescription=extract k8s files\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/tar xzvf /opt/kubernetes.tar.gz -C /\n\n[Install]\nWantedBy=multi-user.target\n",
					Enabled: true,
					Name:    "extractk8s.service",
					Dropins: []types.Dropin{
						{
							Name:    "30-increase-ulimit.conf",
							Content: "[Service]\nLimitMEMLOCK=infinity\n",
						},
					},
				},
			},
			Version: "v1.15.11",
		},
	},
}
