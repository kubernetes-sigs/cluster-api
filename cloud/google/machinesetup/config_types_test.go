/*
Copyright 2018 The Kubernetes Authors.
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

package machinesetup

import (
	"io"
	"reflect"
	"strings"
	"testing"

	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestParseMachineSetupYaml(t *testing.T) {
	testTables := []struct {
		reader      io.Reader
		expectedErr bool
	}{
		{
			reader: strings.NewReader(`items:
- machineParams:
  - os: ubuntu-1710
    roles:
    - Master
    versions:
      kubelet: 1.9.3
      controlPlane: 1.9.3
  - os: ubuntu-1710
    roles:
    - Master
    versions:
      kubelet: 1.9.4
      controlPlane: 1.9.4
  image: projects/ubuntu-os-cloud/global/images/family/ubuntu-1710
  metadata:
    startupScript: |
      #!/bin/bash
- machineParams:
  - os: ubuntu-1710
    roles:
    - Node
    versions:
      kubelet: 1.9.3
  - os: ubuntu-1710
    roles:
    - Node
    versions:
      kubelet: 1.9.4
  image: projects/ubuntu-os-cloud/global/images/family/ubuntu-1710
  metadata:
    startupScript: |
      #!/bin/bash
      echo this is the node config.`),
			expectedErr: false,
		},
		{
			reader:      strings.NewReader("Not valid yaml"),
			expectedErr: true,
		},
	}

	for _, table := range testTables {
		validConfigs, err := parseMachineSetupYaml(table.reader)
		if table.expectedErr {
			if err == nil {
				t.Errorf("An error was not received as expected.")
			}
			if validConfigs != nil {
				t.Errorf("GetMachineSetupConfigs should be nil, got %v", validConfigs)
			}
		}
		if !table.expectedErr {
			if err != nil {
				t.Errorf("Got unexpected error: %s", err)
			}
			if validConfigs == nil {
				t.Errorf("GetMachineSetupConfigs should have been parsed, but was nil")
			}
		}
	}
}

func TestGetYaml(t *testing.T) {
	testTables := []struct {
		validConfigs    ValidConfigs
		expectedStrings []string
		expectedErr     bool
	}{
		{
			validConfigs: ValidConfigs{
				configList: &configList{
					Items: []config{
						{
							Params: []ConfigParams{
								{
									OS:    "ubuntu-1710",
									Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
									Versions: clusterv1.MachineVersionInfo{
										Kubelet:      "1.9.4",
										ControlPlane: "1.9.4",
									},
								},
							},
							Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
							Metadata: Metadata{
								StartupScript: "Master startup script",
							},
						},
						{
							Params: []ConfigParams{
								{
									OS:    "ubuntu-1710",
									Roles: []clustercommon.MachineRole{clustercommon.NodeRole},
									Versions: clusterv1.MachineVersionInfo{
										Kubelet: "1.9.4",
									},
								},
							},
							Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
							Metadata: Metadata{
								StartupScript: "Node startup script",
							},
						},
					},
				},
			},
			expectedStrings: []string{"startupScript: Master startup script", "startupScript: Node startup script"},
			expectedErr:     false,
		},
	}

	for _, table := range testTables {
		yaml, err := table.validConfigs.GetYaml()
		if err == nil && table.expectedErr {
			t.Errorf("An error was not received as expected.")
		}
		if err != nil && !table.expectedErr {
			t.Errorf("Got unexpected error: %s", err)
		}
		for _, expectedString := range table.expectedStrings {
			if !strings.Contains(yaml, expectedString) {
				t.Errorf("Yaml did not contain expected string, got:\n%s\nwant:\n%s", yaml, expectedString)
			}
		}
	}
}

func validConfigs(configs ...config) ValidConfigs {
	return ValidConfigs{
		configList: &configList{
			Items: configs,
		},
	}
}

func TestMatchMachineSetupConfig(t *testing.T) {
	masterMachineSetupConfig := config{
		Params: []ConfigParams{
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.3",
					ControlPlane:     "1.9.3",
				},
			},
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
					ControlPlane:     "1.9.4",
				},
			},
		},
		Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
		Metadata: Metadata{
			StartupScript: "Master startup script",
		},
	}
	nodeMachineSetupConfig := config{
		Params: []ConfigParams{
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.3",
				},
			},
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
				},
			},
		},
		Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
		Metadata: Metadata{
			StartupScript: "Node startup script",
		},
	}
	multiRoleSetupConfig := config{
		Params: []ConfigParams{
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole, clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.5",
					ControlPlane:     "1.9.5",
				},
			},
		},
		Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
		Metadata: Metadata{
			StartupScript: "Multi-role startup script",
		},
	}
	duplicateMasterMachineSetupConfig := config{
		Params: []ConfigParams{
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.3",
					ControlPlane:     "1.9.3",
				},
			},
			{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
					ControlPlane:     "1.9.4",
				},
			},
		},
		Image: "projects/ubuntu-os-cloud/global/images/family/ubuntu-1710",
		Metadata: Metadata{
			StartupScript: "Duplicate master startup script",
		},
	}

	testTables := []struct {
		validConfigs  ValidConfigs
		params        ConfigParams
		expectedMatch *config
		expectedErr   bool
	}{
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
					ControlPlane:     "1.9.4",
				},
			},
			expectedMatch: &masterMachineSetupConfig,
			expectedErr:   false,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
				},
			},
			expectedMatch: &nodeMachineSetupConfig,
			expectedErr:   false,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole, clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.5",
					ControlPlane:     "1.9.5",
				},
			},
			expectedMatch: &multiRoleSetupConfig,
			expectedErr:   false,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.5",
					ControlPlane:     "1.9.5",
				},
			},
			expectedMatch: nil,
			expectedErr:   true,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole, clustercommon.NodeRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.3",
				},
			},
			expectedMatch: nil,
			expectedErr:   true,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole, clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.5",
					ControlPlane:     "1.9.5",
				},
			},
			expectedMatch: nil,
			expectedErr:   true,
		},
		{
			validConfigs: validConfigs(masterMachineSetupConfig, nodeMachineSetupConfig, multiRoleSetupConfig, duplicateMasterMachineSetupConfig),
			params: ConfigParams{
				OS:    "ubuntu-1710",
				Roles: []clustercommon.MachineRole{clustercommon.MasterRole},
				Versions: clusterv1.MachineVersionInfo{
					Kubelet:          "1.9.4",
					ControlPlane:     "1.9.4",
				},
			},
			expectedMatch: nil,
			expectedErr:   true,
		},
	}

	for _, table := range testTables {
		matched, err := table.validConfigs.matchMachineSetupConfig(&table.params)
		if !reflect.DeepEqual(matched, table.expectedMatch) {
			t.Errorf("Matched machine setup config was incorrect, got: %+v,\n want %+v.", matched, table.expectedMatch)
		}
		if err == nil && table.expectedErr {
			t.Errorf("An error was not received as expected.")
		}
		if err != nil && !table.expectedErr {
			t.Errorf("Got unexpected error: %s", err)
		}
	}
}
