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
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/errors"
)

type RegisterOptions struct {
	RegistryEndpoint string
}

var ro = &RegisterOptions{}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a kubernetes cluster with a Cluster Registry",
	Long:  `Register a kubernetes cluster with a Cluster Registry`,
	Run: func(cmd *cobra.Command, args []string) {
		if ro.RegistryEndpoint == "" {
			glog.Error("Please provide yaml file for cluster definition.")
			cmd.Help()
			os.Exit(1)
		}
		if err := RunRegister(ro); err != nil {
			glog.Exit(err)
		}
	},
}

func RunRegister(co *RegisterOptions) error {
	return errors.NotImplementedError
}

func init() {
	registerCmd.Flags().StringVarP(&ro.RegistryEndpoint, "registry-endpoint", "r", "", "registry endpoint")

	RootCmd.AddCommand(registerCmd)
}
