/*
Copyright 2020 The Kubernetes Authors.

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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

const completionBoilerPlate = `# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
`

var (
	completionLong = LongDesc(`
		Output shell completion code for the specified shell (bash, zsh or fish).
		The shell code must be evaluated to provide interactive completion of
		clusterctl commands. This can be done by sourcing it from the
		.bash_profile.`)

	completionExample = Examples(`
		Bash:
		# Install bash completion on macOS using Homebrew
		brew install bash-completion
		printf "\n# Bash completion support\nsource $(brew --prefix)/etc/bash_completion\n" >> $HOME/.bash_profile
		source $HOME/.bash_profile

		# Load the clusterctl completion code for bash into the current shell
		source <(clusterctl completion bash)

		# Write bash completion code to a file and source it from .bash_profile
		clusterctl completion bash > ~/.kube/clusterctl_completion.bash.inc
		printf "\n# clusterctl shell completion\nsource '$HOME/.kube/clusterctl_completion.bash.inc'\n" >> $HOME/.bash_profile
		source $HOME/.bash_profile

		Zsh:
		# If shell completion is not already enabled in your environment you will need
		# to enable it.  You can execute the following once:
		echo "autoload -U compinit; compinit" >> ~/.zshrc

		# To load completions for each session, execute once:
		clusterctl completion zsh > "${fpath[1]}/_clusterctl"

		Fish:
		# To load completions in your current shell, execute the following command:
		clusterctl completion fish | source

		# You will need to start a new shell for this setup to take effect.`)

	completionCmd = &cobra.Command{
		Use:     "completion [bash|zsh|fish]",
		GroupID: groupOther,
		Short:   "Output shell completion code for the specified shell (bash, zsh or fish)",
		Long:    LongDesc(completionLong),
		Example: completionExample,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("please specify a shell")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(os.Stdout, cmd, args[0])
		},
		ValidArgs: GetSupportedShells(),
	}

	completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
		"fish": runCompletionFish,
	}
)

// GetSupportedShells returns a list of supported shells.
func GetSupportedShells() []string {
	shells := []string{}
	for s := range completionShells {
		shells = append(shells, s)
	}
	return shells
}

func init() {
	RootCmd.AddCommand(completionCmd)
}

func runCompletion(out io.Writer, cmd *cobra.Command, shell string) error {
	run, found := completionShells[shell]
	if !found {
		return fmt.Errorf("unsupported shell type %q", shell)
	}

	return run(out, cmd)
}

func runCompletionBash(out io.Writer, cmd *cobra.Command) error {
	fmt.Fprintf(out, "%s\n", completionBoilerPlate)

	return cmd.Root().GenBashCompletion(out)
}

func runCompletionFish(out io.Writer, cmd *cobra.Command) error {
	fmt.Fprintf(out, "%s\n", completionBoilerPlate)

	return cmd.Root().GenFishCompletion(out, true)
}

func runCompletionZsh(out io.Writer, cmd *cobra.Command) error {
	var b bytes.Buffer

	if err := cmd.Root().GenZshCompletion(&b); err != nil {
		return err
	}

	// Insert boilerplate after the first line.
	// The first line of a zsh completion function file must be "#compdef foobar".
	line, err := b.ReadBytes('\n')
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n%s%s\n", string(line), completionBoilerPlate, b.String())

	// Cobra doesn't source zsh completion file, explicitly doing it here
	fmt.Fprintln(out, "compdef _clusterctl clusterctl")

	return nil
}

func contextCompletionFunc(kubeconfigFlag *pflag.Flag) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		configClient, err := config.New(context.Background(), cfgFile)
		if err != nil {
			return completionError(err)
		}

		client := cluster.New(cluster.Kubeconfig{Path: kubeconfigFlag.Value.String()}, configClient)
		comps, err := client.Proxy().GetContexts(toComplete)
		if err != nil {
			return completionError(err)
		}

		return comps, cobra.ShellCompDirectiveNoFileComp
	}
}

func resourceNameCompletionFunc(kubeconfigFlag, contextFlag, namespaceFlag *pflag.Flag, groupVersion, kind string) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		ctx := context.Background()

		configClient, err := config.New(ctx, cfgFile)
		if err != nil {
			return completionError(err)
		}

		clusterClient := cluster.New(cluster.Kubeconfig{Path: kubeconfigFlag.Value.String(), Context: contextFlag.Value.String()}, configClient)

		var namespace string
		if namespaceFlag != nil {
			namespace = namespaceFlag.Value.String()
		}

		if namespace == "" {
			namespace, err = clusterClient.Proxy().CurrentNamespace()
			if err != nil {
				return completionError(err)
			}
		}

		comps, err := clusterClient.Proxy().GetResourceNames(ctx, groupVersion, kind, []client.ListOption{client.InNamespace(namespace)}, toComplete)
		if err != nil {
			return completionError(err)
		}

		return comps, cobra.ShellCompDirectiveNoFileComp
	}
}

func completionError(err error) ([]string, cobra.ShellCompDirective) {
	cobra.CompError(err.Error())
	return nil, cobra.ShellCompDirectiveError
}
