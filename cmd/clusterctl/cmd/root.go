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
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/adrg/xdg"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kubectlcmd "k8s.io/kubectl/pkg/cmd"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

const (
	groupDebug      = "group-debug"
	groupManagement = "group-management"
	groupOther      = "group-other"
)

var (
	cfgFile   string
	verbosity *int
)

// RootCmd is clusterctl root CLI command.
var RootCmd = &cobra.Command{
	Use:          "clusterctl",
	SilenceUsage: true,
	Short:        "clusterctl controls the lifecycle of a Cluster API management cluster",
	Long: LongDesc(`
		Get started with Cluster API using clusterctl to create a management cluster,
		install providers, and create templates for your workload cluster.`),
	PersistentPostRunE: func(*cobra.Command, []string) error {
		ctx := context.Background()

		// Check if clusterctl needs an upgrade "AFTER" running each command
		// and sub-command.
		configClient, err := config.New(ctx, cfgFile)
		if err != nil {
			return err
		}
		disable, err := configClient.Variables().Get("CLUSTERCTL_DISABLE_VERSIONCHECK")
		if err == nil && disable == "true" {
			// version check is disabled. Return early.
			return nil
		}
		checker, err := newVersionChecker(ctx, configClient.Variables())
		if err != nil {
			return err
		}
		output, err := checker.Check(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to verify clusterctl version")
		}
		if output != "" {
			// Print the output in yellow so it is more visible.
			fmt.Fprintf(os.Stderr, "\033[33m%s\033[0m", output)
		}

		configDirectory, err := xdg.ConfigFile(config.ConfigFolderXDG)
		if err != nil {
			return err
		}

		// clean the downloaded config if was fetched from remote
		downloadConfigFile := filepath.Join(configDirectory, config.DownloadConfigFile)
		if _, err := os.Stat(downloadConfigFile); err == nil {
			if verbosity != nil && *verbosity >= 5 {
				fmt.Fprintf(os.Stdout, "Removing downloaded clusterctl config file: %s\n", config.DownloadConfigFile)
			}
			_ = os.Remove(downloadConfigFile)
		}

		return nil
	},
}

// Execute executes the root command.
func Execute() {
	handlePlugins()

	if err := RootCmd.Execute(); err != nil {
		if verbosity != nil && *verbosity >= 5 {
			if err, ok := err.(stackTracer); ok {
				for _, f := range err.StackTrace() {
					fmt.Fprintf(os.Stderr, "%+s:%d\n", f, f)
				}
			}
		}
		// TODO: print cmd help if validation error
		os.Exit(1)
	}
}

func init() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	verbosity = flag.CommandLine.Int("v", 0, "Set the log level verbosity. This overrides the CLUSTERCTL_LOG_LEVEL environment variable.")

	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Path to clusterctl configuration (default is `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml`) or to a remote location (i.e. https://example.com/clusterctl.yaml)")

	RootCmd.AddGroup(
		&cobra.Group{
			ID:    groupManagement,
			Title: "Cluster Management Commands:",
		},
		&cobra.Group{
			ID:    groupDebug,
			Title: "Troubleshooting and Debugging Commands:",
		},
		&cobra.Group{
			ID:    groupOther,
			Title: "Other Commands:",
		})

	RootCmd.SetHelpCommandGroupID(groupOther)
	RootCmd.SetCompletionCommandGroupID(groupOther)

	cobra.OnInitialize(initConfig, registerCompletionFuncForCommonFlags)
}

func initConfig() {
	ctx := context.Background()

	// check if the CLUSTERCTL_LOG_LEVEL was set via env var or in the config file
	if *verbosity == 0 {
		configClient, err := config.New(ctx, cfgFile)
		if err == nil {
			v, err := configClient.Variables().Get("CLUSTERCTL_LOG_LEVEL")
			if err == nil && v != "" {
				verbosityFromEnv, err := strconv.Atoi(v)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to convert CLUSTERCTL_LOG_LEVEL string to an int. err=%s\n", err.Error())
					os.Exit(1)
				}
				verbosity = &verbosityFromEnv
			}
		}
	}

	log := logf.NewLogger(logf.WithThreshold(verbosity))
	logf.SetLogger(log)
	ctrl.SetLogger(log)
}

func registerCompletionFuncForCommonFlags() {
	visitCommands(RootCmd, func(cmd *cobra.Command) {
		if kubeconfigFlag := cmd.Flags().Lookup("kubeconfig"); kubeconfigFlag != nil {
			// context in kubeconfig
			for _, flagName := range []string{"kubeconfig-context", "to-kubeconfig-context"} {
				_ = cmd.RegisterFlagCompletionFunc(flagName, contextCompletionFunc(kubeconfigFlag))
			}

			if contextFlag := cmd.Flags().Lookup("kubeconfig-context"); contextFlag != nil {
				// namespace
				for _, flagName := range []string{"namespace", "target-namespace", "from-config-map-namespace"} {
					_ = cmd.RegisterFlagCompletionFunc(flagName, resourceNameCompletionFunc(kubeconfigFlag, contextFlag, nil, "v1", "namespace"))
				}
			}
		}
	})
}

func handlePlugins() {
	args := os.Args
	pluginHandler := kubectlcmd.NewDefaultPluginHandler([]string{"clusterctl"})
	if len(args) > 1 {
		cmdPathPieces := args[1:]

		// only look for suitable extension executables if
		// the specified command does not already exist
		if _, _, err := RootCmd.Find(cmdPathPieces); err != nil {
			// Also check the commands that will be added by Cobra.
			// These commands are only added once rootCmd.Execute() is called, so we
			// need to check them explicitly here.
			var cmdName string // first "non-flag" arguments
			for _, arg := range cmdPathPieces {
				if !strings.HasPrefix(arg, "-") {
					cmdName = arg
					break
				}
			}

			switch cmdName {
			case "help", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
				// Don't search for a plugin
			default:
				if err := kubectlcmd.HandlePluginCommand(pluginHandler, cmdPathPieces, false); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}
	}
}

const indentation = `  `

// LongDesc normalizes a command's long description to follow the conventions.
func LongDesc(s string) string {
	if s == "" {
		return s
	}
	return normalizer{s}.heredoc().trim().string
}

// Examples normalizes a command's examples to follow the conventions.
func Examples(s string) string {
	if s == "" {
		return s
	}
	return normalizer{s}.trim().indent().string
}

// TODO: document this, what does it do? Why is it here?
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
	splitLines := strings.Split(s.string, "\n")
	indentedLines := make([]string, 0, len(splitLines))
	for _, line := range splitLines {
		trimmed := strings.TrimSpace(line)
		indented := indentation + trimmed
		indentedLines = append(indentedLines, indented)
	}
	s.string = strings.Join(indentedLines, "\n")
	return s
}
