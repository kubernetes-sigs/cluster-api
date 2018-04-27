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

package namedmachines

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
)

// Config Watch holds the path to the named machines yaml file.
type ConfigWatch struct {
	path string
}

// A single named machine.
type NamedMachine struct {
	MachineName string
	MachineHcl  string
}

// A list of named machines.
type NamedMachinesItems struct {
	Items []NamedMachine `json:"items"`
}

// All named machines defined in yaml.
type NamedMachines struct {
	namedMachinesItems *NamedMachinesItems
}

func NewConfigWatch(path string) (*ConfigWatch, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return &ConfigWatch{path: path}, nil
}

// Returns all named machines for ConfigWatch.
func (cw *ConfigWatch) NamedMachines() (*NamedMachines, error) {
	file, err := os.Open(cw.path)
	if err != nil {
		return nil, err
	}
	return parseNamedMachinesYaml(file)
}

func parseNamedMachinesYaml(reader io.Reader) (*NamedMachines, error) {
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	items := &NamedMachinesItems{}
	err = yaml.Unmarshal(bytes, items)
	if err != nil {
		return nil, err
	}

	return &NamedMachines{items}, nil
}

func (nm *NamedMachines) GetYaml() (string, error) {
	bytes, err := yaml.Marshal(nm.namedMachinesItems)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Returns a NamedMachine that matches the passed name.
func (nm *NamedMachines) MatchMachine(machineName string) (*NamedMachine, error) {
	for _, namedMachine := range nm.namedMachinesItems.Items {
		if namedMachine.MachineName == machineName {
			return &namedMachine, nil
		}
	}
	return nil, fmt.Errorf("could not find a machine with name %s", machineName)
}
