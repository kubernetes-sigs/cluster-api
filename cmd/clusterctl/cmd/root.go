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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/internal/templates"
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
	Long: templates.LongDesc(`
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
	pluginHandler := newDefaultPluginHandler([]string{"clusterctl"})
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
				if err := handlePluginCommand(pluginHandler, cmdPathPieces, 0); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}
	}
}

// The following code is inlined from: https://github.com/kubernetes/kubernetes/blob/v1.31.0/staging/src/k8s.io/kubectl/pkg/cmd/cmd.go#L181
// to avoid a dependency on k8s.io/kubectl.

// pluginHandler is capable of parsing command line arguments
// and performing executable filename lookups to search
// for valid plugin files, and execute found plugins.
type pluginHandler interface {
	// Lookup will iterate over a list of given prefixes
	// in order to recognize valid plugin filenames.
	// The first filepath to match a prefix is returned.
	Lookup(filename string) (string, bool)
	// Execute receives an executable's filepath, a slice
	// of arguments, and a slice of environment variables
	// to relay to the executable.
	Execute(executablePath string, cmdArgs, environment []string) error
}

// defaultPluginHandler implements pluginHandler.
type defaultPluginHandler struct {
	ValidPrefixes []string
}

// newDefaultPluginHandler instantiates the defaultPluginHandler with a list of
// given filename prefixes used to identify valid plugin filenames.
func newDefaultPluginHandler(validPrefixes []string) *defaultPluginHandler {
	return &defaultPluginHandler{
		ValidPrefixes: validPrefixes,
	}
}

// Lookup implements pluginHandler.
func (h *defaultPluginHandler) Lookup(filename string) (string, bool) {
	for _, prefix := range h.ValidPrefixes {
		path, err := exec.LookPath(fmt.Sprintf("%s-%s", prefix, filename))
		if shouldSkipOnLookPathErr(err) || path == "" {
			continue
		}
		return path, true
	}
	return "", false
}

func command(name string, arg ...string) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: name,
		Args: append([]string{name}, arg...),
	}
	if filepath.Base(name) == name {
		lp, err := exec.LookPath(name)
		if lp != "" && !shouldSkipOnLookPathErr(err) {
			// Update cmd.Path even if err is non-nil.
			// If err is ErrDot (especially on Windows), lp may include a resolved
			// extension (like .exe or .bat) that should be preserved.
			cmd.Path = lp
		}
	}
	return cmd
}

// Execute implements pluginHandler.
func (h *defaultPluginHandler) Execute(executablePath string, cmdArgs, environment []string) error {
	// Windows does not support exec syscall.
	if runtime.GOOS == "windows" {
		cmd := command(executablePath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = environment
		err := cmd.Run()
		if err == nil {
			os.Exit(0)
		}
		return err
	}

	// invoke cmd binary relaying the environment and args given
	// append executablePath to cmdArgs, as execve will make first argument the "binary name".
	return syscall.Exec(executablePath, append([]string{executablePath}, cmdArgs...), environment) //nolint:gosec // let's keep this code in sync with upstream
}

func shouldSkipOnLookPathErr(err error) bool {
	return err != nil && !errors.Is(err, exec.ErrDot)
}

// handlePluginCommand receives a pluginHandler and command-line arguments and attempts to find
// a plugin executable on the PATH that satisfies the given arguments.
func handlePluginCommand(pluginHandler pluginHandler, cmdArgs []string, minArgs int) error {
	remainingArgs := []string{} // all "non-flag" arguments
	for _, arg := range cmdArgs {
		if strings.HasPrefix(arg, "-") {
			break
		}
		remainingArgs = append(remainingArgs, strings.Replace(arg, "-", "_", -1))
	}

	if len(remainingArgs) == 0 {
		// the length of cmdArgs is at least 1
		return fmt.Errorf("flags cannot be placed before plugin name: %s", cmdArgs[0])
	}

	foundBinaryPath := ""

	// attempt to find binary, starting at longest possible name with given cmdArgs
	for len(remainingArgs) > 0 {
		path, found := pluginHandler.Lookup(strings.Join(remainingArgs, "-"))
		if !found {
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
			if len(remainingArgs) < minArgs {
				// we shouldn't continue searching with shorter names.
				// this is especially for not searching kubectl-create plugin
				// when kubectl-create-foo plugin is not found.
				break
			}

			continue
		}

		foundBinaryPath = path
		break
	}

	if foundBinaryPath == "" {
		return nil
	}

	// invoke cmd binary relaying the current environment and args given
	return pluginHandler.Execute(foundBinaryPath, cmdArgs[len(remainingArgs):], os.Environ())
}
