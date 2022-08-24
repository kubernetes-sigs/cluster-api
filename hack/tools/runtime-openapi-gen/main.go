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

// main is the main package for openapi-gen.
package main

import (
	"encoding/json"
	"os"
	"path"

	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

var (
	version    = flag.String("version", "", "Version for the OpenAPI specification.")
	outputFile = flag.String("output-file", "runtime-sdk-openapi.yaml", "Output file name.")
)

func main() {
	flag.Parse()

	if *version == "" {
		klog.Exit("--version must be specified")
	}
	if *outputFile == "" {
		klog.Exit("--output-file must be specified")
	}

	outputFileExt := path.Ext(*outputFile)
	if outputFileExt != ".yaml" && outputFileExt != ".json" {
		klog.Exit("--output-file must have either 'yaml' or 'json' extension")
	}

	c := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(c)

	c.AddOpenAPIDefinitions(clusterv1.GetOpenAPIDefinitions)
	c.AddOpenAPIDefinitions(GetOpenAPIDefinitions)

	openAPI, err := c.OpenAPI(*version)
	if err != nil {
		klog.Exitf("Failed to generate OpenAPI specification: %v", err)
	}

	var openAPIBytes []byte
	if outputFileExt == ".yaml" {
		openAPIBytes, err = yaml.Marshal(openAPI)
	} else {
		openAPIBytes, err = json.MarshalIndent(openAPI, " ", " ")
	}
	if err != nil {
		klog.Exitf("Failed to marshal OpenAPI specification: %v", err)
	}

	err = os.WriteFile(*outputFile, openAPIBytes, 0600)
	if err != nil {
		klog.Exitf("Failed to write OpenAPI specification to file %q: %v", outputFile, err)
	}
}
