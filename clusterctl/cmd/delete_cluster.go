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
	"sigs.k8s.io/cluster-api/pkg/errors"
)

type DeleteOptions struct {
	ClusterName        string
	ProviderComponents string
}

var do = &DeleteOptions{}

var deleteClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Delete kubernetes cluster",
	Long:  `Delete a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if do.ClusterName == "" {
			exitWithHelp(cmd, "Please provide cluster name.")
		}
		if err := RunDelete(); err != nil {
			glog.Exit(err)
		}
	},
}

func init() {
	deleteClusterCmd.Flags().StringVarP(&do.ProviderComponents, "provider-components", "p", "", "A yaml file containing cluster api provider controllers and supporting objects, if empty the value is loaded from the cluster's configuration store.")
	deleteCmd.AddCommand(deleteClusterCmd)
}

func RunDelete() error {
	return errors.NotImplementedError
}
