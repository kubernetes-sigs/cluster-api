/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"fmt"

	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type options struct {
	script string
}

var opts options

var generateCmd = &cobra.Command{
	Use:   "generate_image",
	Short: "Outputs a script to generate a preloaded image",
	Run: func(cmd *cobra.Command, args []string) {
		if opts.script == "" {
			glog.Error("Please provide a startup script.")
			cmd.Help()
			os.Exit(1)
		}

		if err := runGenerate(opts); err != nil {
			glog.Exit(err)
		}
	},
}

func init() {
	generateCmd.Flags().StringVar(&opts.script, "script", "", "The path to the machine's startup script")
}

func runGenerate(o options) error {
	bytes, err := ioutil.ReadFile(o.script)
	if err != nil {
		return err
	}

	// just print the script for now
	// TODO actually start a VM, let it run the script, stop the VM, then create the image
	fmt.Println(string(bytes))
	return nil
}

func main() {
	if err := generateCmd.Execute(); err != nil {
		glog.Exit(err)
	}
}
