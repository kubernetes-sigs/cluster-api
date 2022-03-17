/*
Copyright 2021 The Kubernetes Authors.

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
	"path/filepath"

	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"
	generatorargs "k8s.io/kube-openapi/cmd/openapi-gen/args"
	"k8s.io/kube-openapi/pkg/generators"
)

var (
	inputDirs          = flag.StringSlice("input-dirs", nil, "Comma-separated list of import paths to get input types from.")
	outputFileBaseName = flag.String("output-file-base", "zz_generated.openapi", "Base name (without .go suffix) for output files.")
	headerFilePath     = flag.String("header-file", "", "File containing boilerplate header text. The string YEAR will be replaced with the current 4-digit year.")
)

func main() {
	flag.Parse()

	if len(*inputDirs) < 1 {
		klog.Exit("--input-dirs must be specified")
	}

	if *headerFilePath == "" {
		klog.Exit("--header-file must be specified")
	}

	for _, d := range *inputDirs {
		fmt.Println("** Generating openapi schema for types in", d, "**")
		genericArgs, customArgs := generatorargs.NewDefaults()
		genericArgs.InputDirs = []string{d}
		genericArgs.OutputBase = filepath.Dir(d)
		genericArgs.OutputPackagePath = filepath.Base(d)
		genericArgs.OutputFileBaseName = *outputFileBaseName
		genericArgs.GoHeaderFilePath = *headerFilePath
		genericArgs.GeneratedByCommentTemplate = ""
		customArgs.ReportFilename = "-" // stdout

		if err := genericArgs.Execute(
			generators.NameSystems(),
			generators.DefaultNameSystem(),
			generators.Packages,
		); err != nil {
			klog.Exit("OpenAPI definition generation error: %v", err)
		}
	}
}
