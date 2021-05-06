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
	"strings"

	"github.com/pkg/errors"

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

// printVariablesOutput prints the expected variables in the template to stdout.
func printVariablesOutput(printer client.YamlPrinter) error {
	if len(printer.Variables()) > 0 {
		fmt.Println("Variables:")
		for _, v := range printer.Variables() {
			fmt.Printf("  - %s\n", v)
		}
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
	fmt.Printf("WatchingNamespace:  %s\n", c.WatchingNamespace())
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
