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
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

type topologyDryRunOptions struct {
	file string
}

var dr = &topologyDryRunOptions{}

var topologyDryRunCmd = &cobra.Command{
	Use:     "topology-dryrun",
	Short:   "",
	Long:    LongDesc(``),
	Example: Examples(``),
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTopologyDryRun()
	},
}

func init() {
	topologyDryRunCmd.Flags().StringVarP(&dr.file, "file", "f", "", "")

	alphaCmd.AddCommand(topologyDryRunCmd)
}

func runTopologyDryRun() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	objs, err := c.DryRunTopology(client.DryRunOptions{
		File: dr.file,
	})
	if err != nil {
		return err
	}

	yaml, err := utilyaml.FromUnstructured(objs)
	if err != nil {
		return err
	}
	yaml = append(yaml, '\n')

	if _, err := os.Stdout.Write(yaml); err != nil {
		return errors.Wrap(err, "failed to write yaml to Stdout")
	}
	return nil
}
