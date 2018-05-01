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

package cmd

import (
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/errors"
)

type CreateOptions struct {
	Cluster string
	Machine string
	ProviderComponents  string
}

var co = &CreateOptions{}

func NewCmdCreateCluster() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Create kubernetes cluster",
		Long:  `Create a kubernetes cluster with one command`,
		Run: func(cmd *cobra.Command, args []string) {
			if co.Cluster == "" {
				exitWithHelp(cmd, "Please provide yaml file for cluster definition.")
			}
			if co.Machine == "" {
				exitWithHelp(cmd, "Please provide yaml file for machine definition.")
			}
			if co.ProviderComponents == "" {
				exitWithHelp(cmd, "Please provide a yaml file for provider component definitions.")
			}
			if err := RunCreate(co); err != nil {
				glog.Exit(err)
			}
		},
	}

	return cmd
}

func RunCreate(co *CreateOptions) error {
	return errors.NotImplementedError
}
