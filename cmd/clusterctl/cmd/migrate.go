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

	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/migrate"
)

type migrateOptions struct {
	output string
}

var migrateOpts = &migrateOptions{}

var migrateCmd = &cobra.Command{
	Use:   "migrate [SOURCE]",
	Short: "Migrate CAPI resources from v1beta1 to v1beta2 [ALPHA]",
	Long: `Migrate CAPI resources from v1beta1 to v1beta2 API versions [ALPHA].

This command is in ALPHA status and currently supports only v1beta1 to v1beta2 migration.

The command reads YAML files containing Cluster API resources and converts v1beta1
resources to v1beta2 (hub) versions. Non-CAPI resources are passed through unchanged.

LIMITATIONS (Alpha Status):
- Only supports v1beta1 to v1beta2 migration
- ClusterClass patches are not migrated (limitation documented)
- Field order may change and comments will be removed in output
- API version references are dropped during conversion (except cluster class and external remediation references)

Examples:
  # Migrate from file to stdout
  clusterctl alpha migrate cluster.yaml

  # Migrate from stdin to stdout
  cat cluster.yaml | clusterctl alpha migrate

  # Migrate to output file
  clusterctl alpha migrate cluster.yaml --output migrated-cluster.yaml

  # Chain with other tools
  clusterctl alpha migrate cluster.yaml | kubectl apply -f -`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrate(args)
	},
}

func init() {
	migrateCmd.Flags().StringVarP(&migrateOpts.output, "output", "o", "", "Output file path (default: stdout)")
}

func runMigrate(args []string) error {
	fmt.Fprintf(os.Stderr, "WARNING: This command is in ALPHA status and supports only v1beta1 to v1beta2 migration.\n")
	fmt.Fprintf(os.Stderr, "See 'clusterctl alpha migrate --help' for current limitations and future roadmap.\n\n")

	// Determine input source
	var input io.Reader
	var inputName string

	if len(args) == 0 {
		input = os.Stdin
		inputName = "stdin"
	} else {
		sourceFile := args[0]
		file, err := os.Open(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to open input file %q: %w", sourceFile, err)
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
			return fmt.Errorf("failed to create output file %q: %w", migrateOpts.output, err)
		}
		defer outputFile.Close()
		output = outputFile
	}

	// TODO
	engine, err := migrate.NewEngine()
	if err != nil {
		return fmt.Errorf("failed to create migration engine: %w", err)
	}

	opts := migrate.MigrationOptions{
		Input:  input,
		Output: output,
		Errors: os.Stderr,
	}

	result, err := engine.Migrate(opts)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
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
		return fmt.Errorf("migration completed with %d errors", result.ErrorCount)
	}

	return nil
}
