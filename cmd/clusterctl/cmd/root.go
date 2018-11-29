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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

var RootCmd = &cobra.Command{
	Use:   "clusterctl",
	Short: "cluster management",
	Long:  `Simple kubernetes cluster management`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Glog requires this otherwise it complains.
		flag.CommandLine.Parse(nil)

		// This is a temporary hack to enable proper logging until upstream dependencies
		// are migrated to fully utilize klog instead of glog.
		klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
		klog.InitFlags(klogFlags)

		// Sync the glog and klog flags.
		cmd.Flags().VisitAll(func(f1 *pflag.Flag) {
			f2 := klogFlags.Lookup(f1.Name)
			if f2 != nil {
				value := f1.Value.String()
				f2.Value.Set(value)
			}
		})
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func exitWithHelp(cmd *cobra.Command, err string) {
	fmt.Fprintln(os.Stderr, err)
	cmd.Help()
	os.Exit(1)
}

func init() {
	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	InitLogs()
}
