/*
Copyright 2019 The Kubernetes Authors.

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
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

var cfgFile string

var RootCmd = &cobra.Command{
	Use:   "clusterctl",
	Short: "clusterctl controls a management cluster for Cluster API",
	Long: LongDesc(`
		Get started with Cluster API using clusterctl for initializing a management cluster by installing
		Cluster API providers, and then use clusterctl for creating yaml templates for your workload clusters.`),
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		//TODO: print error stack if log v>0
		//TODO: print cmd help if validation error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {

	klog.InitFlags(flag.CommandLine)
	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	// hiding all the klog flags except
	// --log_dir
	// --log_file
	// --log_file_max_size
	// -v, --v Level

	RootCmd.PersistentFlags().MarkHidden("alsologtostderr")  //nolint
	RootCmd.PersistentFlags().MarkHidden("log_backtrace_at") //nolint
	RootCmd.PersistentFlags().MarkHidden("logtostderr")      //nolint
	RootCmd.PersistentFlags().MarkHidden("stderrthreshold")  //nolint
	RootCmd.PersistentFlags().MarkHidden("vmodule")          //nolint
	RootCmd.PersistentFlags().MarkHidden("skip_log_headers") //nolint
	RootCmd.PersistentFlags().MarkHidden("skip_headers")     //nolint
	RootCmd.PersistentFlags().MarkHidden("add_dir_header")   //nolint

	// makes logs look nicer for a CLI app
	RootCmd.PersistentFlags().Set("skip_headers", "true") //nolint
	RootCmd.PersistentFlags().Set("logtostderr", "true")  //nolint

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to the the clusterctl config file (default is $HOME/.clusterctl.yaml)")
}

const Indentation = `  `

// LongDesc normalizes a command's long description to follow the conventions.
func LongDesc(s string) string {
	if len(s) == 0 {
		return s
	}
	return normalizer{s}.heredoc().trim().string
}

// Examples normalizes a command's examples to follow the conventions.
func Examples(s string) string {
	if len(s) == 0 {
		return s
	}
	return normalizer{s}.trim().indent().string
}

type normalizer struct {
	string
}

func (s normalizer) heredoc() normalizer {
	s.string = heredoc.Doc(s.string)
	return s
}

func (s normalizer) trim() normalizer {
	s.string = strings.TrimSpace(s.string)
	return s
}

func (s normalizer) indent() normalizer {
	var indentedLines []string //nolint
	for _, line := range strings.Split(s.string, "\n") {
		trimmed := strings.TrimSpace(line)
		indented := Indentation + trimmed
		indentedLines = append(indentedLines, indented)
	}
	s.string = strings.Join(indentedLines, "\n")
	return s
}
