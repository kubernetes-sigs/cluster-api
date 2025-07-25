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

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/internal/templates"
)

type checkMetadataOptions struct {
	file    string
	version string
}

var (
	checkMetadataOpts = &checkMetadataOptions{}

	checkMetadataLong = templates.LongDesc(`
		Command that will check that the given metadata file contains an entry
		for the supplied version number.

		This is intended to be used in CI to fail a release build early if
		there is no metadata entry for the release version series.
	`)

	checkMetadataExample = templates.Examples(`
		clusterctl check-metadata -f metadata.yaml --version v1.0.17
	`)

	checkMetadataCmd = &cobra.Command{
		Use:     "check-metadata",
		GroupID: groupOther,
		Short:   "Validate the metadata file given a version number",
		Long:    templates.LongDesc(checkMetadataLong),
		Example: checkMetadataExample,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCheckMetadata()
		},
	}
)

func init() {
	checkMetadataCmd.Flags().StringVarP(&checkMetadataOpts.file, "file", "f", "", "Path to metadata.yaml file to check")
	checkMetadataCmd.Flags().StringVar(&checkMetadataOpts.version, "version", "", "The version number to check against the metadata file")

	_ = checkMetadataCmd.MarkFlagRequired("file")
	_ = checkMetadataCmd.MarkFlagRequired("version")

	RootCmd.AddCommand(checkMetadataCmd)
}

func runCheckMetadata() error {
	versionToCheck := strings.TrimPrefix(checkMetadataOpts.version, "v")

	ver, err := semver.Parse(versionToCheck)
	if err != nil {
		return fmt.Errorf("failed parsing %s as semver version: %w", checkMetadataOpts.version, err)
	}

	fileData, err := os.ReadFile(checkMetadataOpts.file)
	if err != nil {
		return fmt.Errorf("failed reading file %s: %w", checkMetadataOpts.file, err)
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
			fmt.Printf("Success, there is a release series defined in the metadata file\n")
			return nil
		}
	}

	return fmt.Errorf("failed to find release series %s in metadata files %s", "", checkMetadataOpts.file)
}
