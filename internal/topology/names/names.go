/*
Copyright 2023 The Kubernetes Authors.

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

// Package names implements name generators for managed topology.
package names

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/pkg/errors"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

// This is a copy of the constants at k8s.io/apiserver/pkg/storage/names.
const (
	maxNameLength          = 63
	randomLength           = 5
	maxGeneratedNameLength = maxNameLength - randomLength
)

type simpleNameGenerator struct {
	base string
}

func (s *simpleNameGenerator) GenerateName() (string, error) {
	base := s.base
	if len(base) > maxGeneratedNameLength {
		base = base[:maxGeneratedNameLength]
	}
	return fmt.Sprintf("%s%s", base, utilrand.String(randomLength)), nil
}

// NameGenerator generates names for objects.
type NameGenerator interface {
	// GenerateName generates a valid name. The generator is responsible for
	// knowing the maximum valid name length.
	GenerateName() (string, error)
}

// SimpleNameGenerator returns a NameGenerator which is based on
// k8s.io/apiserver/pkg/storage/names.SimpleNameGenerator.
func SimpleNameGenerator(base string) NameGenerator {
	return &simpleNameGenerator{
		base: base,
	}
}

// ControlPlaneNameGenerator returns a generator for creating a control plane name.
func ControlPlaneNameGenerator(templateString, clusterName string) NameGenerator {
	return newTemplateGenerator(templateString, clusterName,
		map[string]interface{}{})
}

// MachineDeploymentNameGenerator returns a generator for creating a machinedeployment name.
func MachineDeploymentNameGenerator(templateString, clusterName, topologyName string) NameGenerator {
	return newTemplateGenerator(templateString, clusterName,
		map[string]interface{}{
			"machineDeployment": map[string]interface{}{
				"topologyName": topologyName,
			},
		})
}

// MachinePoolNameGenerator returns a generator for creating a machinepool name.
func MachinePoolNameGenerator(templateString, clusterName, topologyName string) NameGenerator {
	return newTemplateGenerator(templateString, clusterName,
		map[string]interface{}{
			"machinePool": map[string]interface{}{
				"topologyName": topologyName,
			},
		})
}

// templateGenerator parses the template string as text/template and executes it using
// the passed data to generate a name.
type templateGenerator struct {
	template string
	data     map[string]interface{}
}

func newTemplateGenerator(template, clusterName string, data map[string]interface{}) NameGenerator {
	data["cluster"] = map[string]interface{}{
		"name": clusterName,
	}
	data["random"] = utilrand.String(randomLength)

	return &templateGenerator{
		template: template,
		data:     data,
	}
}

func (g *templateGenerator) GenerateName() (string, error) {
	tpl, err := template.New("template name generator").Option("missingkey=error").Parse(g.template)
	if err != nil {
		return "", errors.Wrapf(err, "parsing template %q", g.template)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, g.data); err != nil {
		return "", errors.Wrap(err, "rendering template")
	}

	name := buf.String()

	// If the name exceeds the maxNameLength: trim to maxGeneratedNameLength and add
	// a random suffix.
	if len(name) > maxNameLength {
		name = name[:maxGeneratedNameLength] + utilrand.String(randomLength)
	}

	return name, nil
}

// BootstrapTemplateNamePrefix calculates the name prefix for a BootstrapTemplate.
func BootstrapTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-", clusterName, machineDeploymentTopologyName)
}

// InfrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func InfrastructureMachineTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-", clusterName, machineDeploymentTopologyName)
}

// BootstrapConfigNamePrefix calculates the name prefix for a BootstrapConfig.
func BootstrapConfigNamePrefix(clusterName, machinePoolTopologyName string) string {
	return fmt.Sprintf("%s-%s-", clusterName, machinePoolTopologyName)
}

// InfrastructureMachinePoolNamePrefix calculates the name prefix for a InfrastructureMachinePool.
func InfrastructureMachinePoolNamePrefix(clusterName, machinePoolTopologyName string) string {
	return fmt.Sprintf("%s-%s-", clusterName, machinePoolTopologyName)
}

// ControlPlaneInfrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func ControlPlaneInfrastructureMachineTemplateNamePrefix(clusterName string) string {
	return fmt.Sprintf("%s-", clusterName)
}
