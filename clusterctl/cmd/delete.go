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
	"os"
	"sigs.k8s.io/cluster-api/errors"
)

type DeleteOptions struct {
	ClusterName string
}

var do = &DeleteOptions{}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete kubernetes cluster",
	Long:  `Delete a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if do.ClusterName == "" {
			glog.Error("Please provide cluster name.")
			cmd.Help()
			os.Exit(1)
		}
		if err := RunDelete(); err != nil {
			glog.Exit(err)
		}
	},
}

func RunDelete() error {
	return errors.NotImplementedError
}

func init() {
	deleteCmd.Flags().StringVarP(&do.ClusterName, "cluster-name", "n", "", "cluster name")

	RootCmd.AddCommand(deleteCmd)
}
