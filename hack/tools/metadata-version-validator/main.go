/*
Copyright 2025 The Kubernetes Authors.

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

// main is the main package for metadata-version-validator.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

func main() {
	file := flag.String("file", "metadata.yaml", "Path to metadata.yaml file to check")
	version := flag.String("version", "", "The version number to check against the metadata file")

	flag.Parse()

	if *file == "" {
		klog.Exit("--file must be specified")
	}
	if *version == "" {
		klog.Exit("--version must be specified")
	}

	if err := runCheckMetadata(*file, *version); err != nil {
		klog.Exitf("failed checking metadata: %v", err)
	}
}

func runCheckMetadata(file, version string) error {
	versionToCheck := strings.TrimPrefix(version, "v")

	ver, err := semver.Parse(versionToCheck)
	if err != nil {
		return fmt.Errorf("failed parsing %s as semver version: %w", version, err)
	}

	fileData, err := os.ReadFile(file) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed reading file %s: %w", file, err)
	}

	scheme := runtime.NewScheme()
	if err := clusterctlv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add metadata type to scheme: %w", err)
	}
	metadata := &clusterctlv1.Metadata{}
	codecFactory := serializer.NewCodecFactory(scheme)

	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), fileData, metadata); err != nil {
		return fmt.Errorf("failed decoding metadata from file: %w", err)
	}

	for _, series := range metadata.ReleaseSeries {
		if series.Major == int32(ver.Major) && series.Minor == int32(ver.Minor) {
			klog.Info("Success, there is a release series defined in the metadata file")
			return nil
		}
	}

	return fmt.Errorf("failed to find release series for version %s in metadata file %s", version, file)
}
