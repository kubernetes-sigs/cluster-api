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
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/migrate"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

type migrateOptions struct {
	output    string
	toVersion string
}

var migrateOpts = &migrateOptions{}

var supportedTargetVersions = []string{
	clusterv1.GroupVersion.Version,
}

var migrateCmd = &cobra.Command{
	Use:   "migrate [SOURCE]",
	Short: "EXPERIMENTAL: Migrate cluster.x-k8s.io resources between API versions",
	Long: `EXPERIMENTAL: Migrate cluster.x-k8s.io resources between API versions.

This command is EXPERIMENTAL and may be removed in a future release!

Scope and limitations:
- Only cluster.x-k8s.io resources are converted
- Other CAPI API groups are passed through unchanged
- ClusterClass patches are not migrated
- Field order may change and comments will be removed in output
- API version references are dropped during conversion (except ClusterClass and external
  remediation references)

Examples:
  # Migrate from file to stdout
  clusterctl migrate cluster.yaml

  # Migrate from stdin to stdout
  cat cluster.yaml | clusterctl migrate

  # Explicitly specify target <VERSION>
  clusterctl migrate cluster.yaml --to-version <VERSION> --output migrated-cluster.yaml`,

	Args: cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runMigrate(args)
	},
}

func init() {
	migrateCmd.Flags().StringVarP(&migrateOpts.output, "output", "o", "", "Output file path (default: stdout)")
	migrateCmd.Flags().StringVar(&migrateOpts.toVersion, "to-version", clusterv1.GroupVersion.Version, fmt.Sprintf("Target API version for migration (supported: %s)", strings.Join(supportedTargetVersions, ", ")))

	RootCmd.AddCommand(migrateCmd)
}

func isSupportedTargetVersion(version string) bool {
	for _, v := range supportedTargetVersions {
		if v == version {
			return true
		}
	}
	return false
}

func runMigrate(args []string) error {
	if !isSupportedTargetVersion(migrateOpts.toVersion) {
		return errors.Errorf("invalid --to-version value %q: supported versions are %s", migrateOpts.toVersion, strings.Join(supportedTargetVersions, ", "))
	}

	fmt.Fprint(os.Stderr, "WARNING: This command is EXPERIMENTAL and may be removed in a future release!")

	var input io.Reader
	var inputName string

	if len(args) == 0 {
		input = os.Stdin
		inputName = "stdin"
	} else {
		sourceFile := args[0]
		// #nosec G304
		// command accepts user-provided file path by design
		file, err := os.Open(sourceFile)
		if err != nil {
			return errors.Wrapf(err, "failed to open input file %q", sourceFile)
		}
		defer file.Close()
		input = file
		inputName = sourceFile
	}

	// Determine output destination
	var output io.Writer
	var outputFile *os.File
	var err error

	if migrateOpts.output == "" {
		output = os.Stdout
	} else {
		outputFile, err = os.Create(migrateOpts.output)
		if err != nil {
			return errors.Wrapf(err, "failed to create output file %q", migrateOpts.output)
		}
		defer outputFile.Close()
		output = outputFile
	}

	// Create migration engine components
	parser := migrate.NewYAMLParser(scheme.Scheme)

	targetGV := schema.GroupVersion{
		Group:   clusterv1.GroupVersion.Group,
		Version: migrateOpts.toVersion,
	}

	converter, err := migrate.NewConverter(targetGV)
	if err != nil {
		return errors.Wrap(err, "failed to create converter")
	}

	engine, err := migrate.NewEngine(parser, converter)
	if err != nil {
		return errors.Wrap(err, "failed to create migration engine")
	}

	opts := migrate.MigrationOptions{
		Input:     input,
		Output:    output,
		Errors:    os.Stderr,
		ToVersion: migrateOpts.toVersion,
	}

	result, err := engine.Migrate(opts)
	if err != nil {
		return errors.Wrap(err, "migration failed")
	}

	if result.TotalResources > 0 {
		fmt.Fprintf(os.Stderr, "\nMigration completed:\n")
		fmt.Fprintf(os.Stderr, "  Total resources processed: %d\n", result.TotalResources)
		fmt.Fprintf(os.Stderr, "  Resources converted: %d\n", result.ConvertedCount)
		fmt.Fprintf(os.Stderr, "  Resources skipped: %d\n", result.SkippedCount)

		if result.ErrorCount > 0 {
			fmt.Fprintf(os.Stderr, "  Resources with errors: %d\n", result.ErrorCount)
		}

		if len(result.Warnings) > 0 {
			fmt.Fprintf(os.Stderr, "  Warnings: %d\n", len(result.Warnings))
		}

		fmt.Fprintf(os.Stderr, "\nSource: %s\n", inputName)
		if migrateOpts.output != "" {
			fmt.Fprintf(os.Stderr, "Output: %s\n", migrateOpts.output)
		}
	}

	if result.ErrorCount > 0 {
		return errors.Errorf("migration completed with %d errors", result.ErrorCount)
	}

	return nil
}
