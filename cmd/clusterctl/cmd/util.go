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
