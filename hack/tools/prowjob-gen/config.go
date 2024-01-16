/*
Copyright 2024 The Kubernetes Authors.

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

package main

// ProwIgnoredConfig is the top-level configuration struct. Because we want to
// store the configuration in test-infra as yaml file, we have to prevent prow
// from trying to parse our configuration as prow configuration. Prow provides
// the well-known `prow_ignored` key which is not parsed further by Prow.
type ProwIgnoredConfig struct {
	ProwIgnored Config `json:"prow_ignored"`
}

// Config is the configuration file struct.
type Config struct {
	Branches  map[string]BranchConfig `json:"branches"`
	Templates []Template              `json:"templates"`
	Versions  VersionsMapper          `json:"versions"`
}

// BranchConfig is the branch-based configuration struct.
type BranchConfig struct {
	Interval                            string     `json:"interval"`
	UpgradesInterval                    string     `json:"upgradesInterval"`
	TestImage                           string     `json:"testImage"`
	KubernetesVersionManagement         string     `json:"kubernetesVersionManagement"`
	KubebuilderEnvtestKubernetesVersion string     `json:"kubebuilderEnvtestKubernetesVersion"`
	Upgrades                            []*Upgrade `json:"upgrades"`
}

// Template refers a template file and defines the target file name template.
type Template struct {
	Template string `json:"template"`
	Name     string `json:"name"`
}

// Upgrade describes a kubernetes upgrade.
type Upgrade struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// VersionsMapper provides key value pairs for a parent key.
type VersionsMapper map[string]map[string]string
