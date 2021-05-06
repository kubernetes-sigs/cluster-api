/*
Copyright 2021 The Kubernetes Authors.

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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

// printYamlOutput prints the yaml content of a generated template to stdout.
func printYamlOutput(printer client.YamlPrinter) error {
	yaml, err := printer.Yaml()
	if err != nil {
		return err
	}
	yaml = append(yaml, '\n')

	if _, err := os.Stdout.Write(yaml); err != nil {
		return errors.Wrap(err, "failed to write yaml to Stdout")
	}
	return nil
}

func stringPtr(s string) *string {
	return &s
}

// printVariablesOutput prints the expected variables in the template to stdout.
func printVariablesOutput(cmd *cobra.Command, template client.Template) error {
	// Decorate the variable map for printing
	variableMap := template.VariableMap()
	for name, defaultValue := range variableMap {
		if defaultValue != nil {
			v := *defaultValue
			// Add quotes around any unquoted strings
			if len(v) > 0 && !strings.HasPrefix(v, "\"") {
				v = fmt.Sprintf("\"%s\"", v)
				variableMap[name] = &v
			}
		}
	}
	// Add variables that are defaulted from clusterctl
	controlPlaneMachineCount, err := cmd.Flags().GetInt64("control-plane-machine-count")
	if err != nil {
		return err
	}
	variableMap["CONTROL_PLANE_MACHINE_COUNT"] = stringPtr(strconv.FormatInt(controlPlaneMachineCount, 10))
	workerMachineCount, err := cmd.Flags().GetInt64("worker-machine-count")
	if err != nil {
		return err
	}
	variableMap["WORKER_MACHINE_COUNT"] = stringPtr(strconv.FormatInt(workerMachineCount, 10))
	variableMap["KUBERNETES_VERSION"] = stringPtr("the value of --kubernetes-version")
	variableMap["CLUSTER_NAME"] = stringPtr("<name> from \"clusterctl config cluster <name>\"")

	// transform variable map into required and optional lists
	var requiredVariables []string
	var optionalVariables []string
	for name, defaultValue := range variableMap {
		if defaultValue != nil {
			optionalVariables = append(optionalVariables, name)
		} else {
			requiredVariables = append(requiredVariables, name)
		}
	}
	sort.Strings(requiredVariables)
	sort.Strings(optionalVariables)

	if len(requiredVariables) > 0 {
		fmt.Println("Required Variables:")
		for _, v := range requiredVariables {
			fmt.Printf("  - %s\n", v)
		}
	}

	if len(optionalVariables) > 0 {
		fmt.Println("\nOptional Variables:")
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', tabwriter.FilterHTML)
		for _, v := range optionalVariables {
			fmt.Fprintf(w, "  - %s\t(defaults to %s)\n", v, *variableMap[v])
		}
		w.Flush()
	}

	fmt.Println()
	return nil
}

// printComponentsAsText prints information about the components to stdout.
func printComponentsAsText(c client.Components) error {
	dir, file := filepath.Split(c.URL())
	// Remove the version suffix from the URL since we already display it
	// separately.
	baseURL, _ := filepath.Split(strings.TrimSuffix(dir, "/"))
	fmt.Printf("Name:               %s\n", c.Name())
	fmt.Printf("Type:               %s\n", c.Type())
	fmt.Printf("URL:                %s\n", baseURL)
	fmt.Printf("Version:            %s\n", c.Version())
	fmt.Printf("File:               %s\n", file)
	fmt.Printf("TargetNamespace:    %s\n", c.TargetNamespace())
	if len(c.Variables()) > 0 {
		fmt.Println("Variables:")
		for _, v := range c.Variables() {
			fmt.Printf("  - %s\n", v)
		}
	}
	if len(c.Images()) > 0 {
		fmt.Println("Images:")
		for _, v := range c.Images() {
			fmt.Printf("  - %s\n", v)
		}
	}
	fmt.Println()

	return nil
}
