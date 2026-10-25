/*
Copyright 2024 The Kubernetes Authors.

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

// main is the main package for prowjob-gen.
package main

import (
	"flag"

	"k8s.io/klog/v2"
)

var (
	configFile   = flag.String("config", "", "Path to the config file")
	outputDir    = flag.String("output-dir", "", "Path to the directory to create the files in")
	templatesDir = flag.String("templates-dir", "", "Path to the directory containing the template files referenced inside the config file")
)

func main() {
	// Parse flags and validate input.
	flag.Parse()
	if *configFile == "" {
		klog.Fatal("Expected flag \"config\" to be set")
	}
	if *outputDir == "" {
		klog.Fatal("Expected flag \"output-dir\" to be set")
	}
	if *templatesDir == "" {
		klog.Fatal("Expected flag \"templates-dir\" to be set")
	}

	// Initialize a generator using the config data.
	g, err := newGenerator(*configFile, *templatesDir, *outputDir)
	if err != nil {
		klog.Fatalf("Failed to initialize generator: %v", err)
	}

	// Cleanup old files.
	if err := g.cleanup(); err != nil {
		klog.Fatalf("Failed to cleanup old generated files: %v", err)
	}

	// Generate new files.
	if err := g.generate(); err != nil {
		klog.Fatalf("Failed to generate prowjobs: %v", err)
	}
}
