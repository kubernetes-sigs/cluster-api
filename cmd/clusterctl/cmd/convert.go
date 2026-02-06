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
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/convert"
)

type convertOptions struct {
	output    string
	toVersion string
}

var convertOpts = &convertOptions{}

var convertCmd = &cobra.Command{
	Use:   "convert [SOURCE]",
	Short: "EXPERIMENTAL: Convert Cluster API resources between API versions",
	Long: `EXPERIMENTAL: Convert Cluster API resources between API versions.

This command is EXPERIMENTAL and may be removed in a future release!

Scope and limitations:
- Only cluster.x-k8s.io resources are converted
- Other CAPI API groups are passed through unchanged
- ClusterClass patches are not converted
- Field order may change and comments will be removed in output
- API version references are dropped during conversion (except ClusterClass and external
  remediation references)

Examples:
  # Convert from file to stdout
  clusterctl convert cluster.yaml

  # Convert from stdin to stdout
  cat cluster.yaml | clusterctl convert

  # Explicitly specify target <VERSION>
  clusterctl convert cluster.yaml --to-version <VERSION> --output converted-cluster.yaml`,

	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runConvert(args)
	},
}

func init() {
	convertCmd.Flags().StringVarP(&convertOpts.output, "output", "o", "", "Output file path (default: stdout)")
	convertCmd.Flags().StringVar(&convertOpts.toVersion, "to-version", clusterv1.GroupVersion.Version, fmt.Sprintf("Target API version for conversion. (Supported versions are: %s)", strings.Join(convert.SupportedTargetVersions, ", ")))

	RootCmd.AddCommand(convertCmd)
}

func isSupportedTargetVersion(version string) bool {
	for _, v := range convert.SupportedTargetVersions {
		if v == version {
			return true
		}
	}
	return false
}

func runConvert(args []string) error {
	if !isSupportedTargetVersion(convertOpts.toVersion) {
		return errors.Errorf("invalid --to-version value %q. Supported versions are %s", convertOpts.toVersion, strings.Join(convert.SupportedTargetVersions, ", "))
	}

	fmt.Fprintln(os.Stderr, "WARNING: This command is EXPERIMENTAL and may be removed in a future release!")

	var inputBytes []byte
	var inputName string
	var err error

	if len(args) == 0 {
		inputBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "failed to read from stdin")
		}
		inputName = "stdin"
	} else {
		sourceFile := args[0]
		// #nosec G304
		// command accepts user-provided file path by design.
		inputBytes, err = os.ReadFile(sourceFile)
		if err != nil {
			return errors.Wrapf(err, "failed to read input file %q", sourceFile)
		}
		inputName = sourceFile
	}

	ctx := context.Background()
	c, err := client.New(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to create clusterctl client")
	}

	result, err := c.Convert(ctx, client.ConvertOptions{
		Input:     inputBytes,
		ToVersion: convertOpts.toVersion,
	})
	if err != nil {
		return errors.Wrap(err, "conversion failed")
	}

	if convertOpts.output == "" {
		_, err = os.Stdout.Write(result.Output)
		if err != nil {
			return errors.Wrap(err, "failed to write to stdout")
		}
	} else {
		err = os.WriteFile(convertOpts.output, result.Output, 0600)
		if err != nil {
			return errors.Wrapf(err, "failed to write output file %q", convertOpts.output)
		}
	}

	if len(result.Messages) > 0 {
		fmt.Fprintln(os.Stderr, "\nConversion messages:")
		for _, msg := range result.Messages {
			fmt.Fprintln(os.Stderr, "  ", msg)
		}
	}

	fmt.Fprintf(os.Stderr, "\nConversion completed successfully\n")
	fmt.Fprintf(os.Stderr, "Source: %s\n", inputName)
	if convertOpts.output != "" {
		fmt.Fprintf(os.Stderr, "Output: %s\n", convertOpts.output)
	} else {
		fmt.Fprintf(os.Stderr, "Output: stdout\n")
	}

	return nil
}
