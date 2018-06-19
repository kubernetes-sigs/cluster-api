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

var validateClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Validate a cluster created by cluster API.",
	Long:  `Validate a cluster created by cluster API.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunValidateCluster(); err != nil {
			glog.Exit(err)
		}
	},
}

func init() {
	validateCmd.AddCommand(validateClusterCmd)
}

func RunValidateCluster() error {
	return errors.NotImplementedError
}
