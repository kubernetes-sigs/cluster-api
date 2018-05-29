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
	"flag"
	"os"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/util/logs"
)

var RootCmd = &cobra.Command{
	Use:   "clusterctl",
	Short: "cluster management",
	Long:  `Simple kubernetes cluster management`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		glog.Exit(err)
	}
}

func exitWithHelp(cmd *cobra.Command, err string) {
	glog.Error(err)
	cmd.Help()
	os.Exit(1)
}

func init() {
	flag.CommandLine.Parse([]string{})

	// Honor glog flags for verbosity control
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	logs.InitLogs()
}
