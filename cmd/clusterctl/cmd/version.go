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

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/yaml"
)

// Version provides the version information of clusterctl.
type Version struct {
	ClientVersion *version.Info `json:"clusterctl"`
}

type versionOptions struct {
	output string
}

var vo = &versionOptions{}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print clusterctl version.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVersion()
	},
}

func init() {
	versionCmd.Flags().StringVarP(&vo.output, "output", "o", "", "Output format; available options are 'yaml', 'json' and 'short'")

	RootCmd.AddCommand(versionCmd)
}

func runVersion() error {
	clientVersion := version.Get()
	v := Version{
		ClientVersion: &clientVersion,
	}

	switch vo.output {
	case "":
		fmt.Printf("clusterctl version: %#v\n", v.ClientVersion)
	case "short":
		fmt.Printf("%s\n", v.ClientVersion.GitVersion)
	case "yaml":
		y, err := yaml.Marshal(&v)
		if err != nil {
			return err
		}
		fmt.Print(string(y))
	case "json":
		y, err := json.MarshalIndent(&v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(y))
	default:
		return errors.Errorf("invalid output format: %s", vo.output)
	}

	return nil
}
