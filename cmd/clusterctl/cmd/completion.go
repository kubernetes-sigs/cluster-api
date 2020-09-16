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
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const completionBoilerPlate = `
# Copyright 2020 The Kubernetes Authors.
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
		Output shell completion code for the specified shell (bash or zsh).
		The shell code must be evaluated to provide interactive completion of
		clusterctl commands. This can be done by sourcing it from the
		.bash_profile.

		Note: this requires the bash-completion framework.

		To install it on macOS use Homebrew:
		    $ brew install bash-completion
		Once installed, bash_completion must be evaluated. This can be done by
		adding the following line to the .bash_profile
		    [[ -r "$(brew --prefix)/etc/profile.d/bash_completion.sh" ]] && . "$(brew --prefix)/etc/profile.d/bash_completion.sh"

		If bash-completion is not installed on Linux, please install the
		'bash-completion' package via your distribution's package manager.

		Note for zsh users: [1] zsh completions are only supported in versions of zsh >= 5.2`)

	completionExample = Examples(`
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

		# Load the clusterctl completion code for zsh[1] into the current shell
		source <(clusterctl completion zsh)`)

	completionCmd = &cobra.Command{
		Use:       "completion [bash|zsh]",
		Short:     "Output shell completion code for the specified shell (bash or zsh)",
		Long:      LongDesc(completionLong),
		Example:   completionExample,
		Args:      cobra.ExactArgs(1),
		RunE:      runCompletion,
		ValidArgs: GetSupportedShells(),
	}

	completionShells = map[string]func(cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
	}
)

// GetSupportedShells returns a list of supported shells
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

func runCompletion(cmd *cobra.Command, args []string) error {

	run, found := completionShells[args[0]]
	if !found {
		return fmt.Errorf("unsupported shell type %q", args[0])
	}

	return run(cmd.Parent())
}

func runCompletionBash(cmd *cobra.Command) error {
	return cmd.GenBashCompletion(os.Stdout)
}

const (
	completionZshHead = "#compdef clusterctl\n"

	completionZshInitialization = `
__clusterctl_bash_source() {
	alias shopt=':'
	emulate -L sh
	setopt kshglob noshglob braceexpand
	source "$@"
}
__clusterctl_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift
		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__clusterctl_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}
__clusterctl_compgen() {
	local completions w
	completions=( $(compgen "$@") ) || return $?
	# filter by given word as prefix
	while [[ "$1" = -* && "$1" != -- ]]; do
		shift
		shift
	done
	if [[ "$1" == -- ]]; then
		shift
	fi
	for w in "${completions[@]}"; do
		if [[ "${w}" = "$1"* ]]; then
			echo "${w}"
		fi
	done
}
__clusterctl_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}
__clusterctl_ltrim_colon_completions()
{
	if [[ "$1" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
		# Remove colon-word prefix from COMPREPLY items
		local colon_word=${1%${1##*:}}
		local i=${#COMPREPLY[*]}
		while [[ $((--i)) -ge 0 ]]; do
			COMPREPLY[$i]=${COMPREPLY[$i]#"$colon_word"}
		done
	fi
}
__clusterctl_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}
__clusterctl_filedir() {
	# Don't need to do anything here.
	# Otherwise we will get trailing space without "compopt -o nospace"
	true
}
autoload -U +X bashcompinit && bashcompinit
# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q 'GNU\|BusyBox'; then
	LWORD='\<'
	RWORD='\>'
fi
__clusterctl_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/_get_comp_words_by_ref "\$@"/_get_comp_words_by_ref "\$*"/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__clusterctl_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__clusterctl_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__clusterctl_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__clusterctl_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__clusterctl_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/builtin declare/g" \
	-e "s/\\\$(type${RWORD}/\$(__clusterctl_type/g" \
	<<'BASH_COMPLETION_EOF'
`

	completionZshTail = `
BASH_COMPLETION_EOF
}
__clusterctl_bash_source <(__clusterctl_convert_bash_to_zsh)
`
)

func runCompletionZsh(cmd *cobra.Command) error {
	fmt.Print(completionZshHead)
	fmt.Print(completionBoilerPlate)
	fmt.Print(completionZshInitialization)

	if err := cmd.GenBashCompletion(os.Stdout); err != nil {
		return err
	}

	fmt.Print(completionZshTail)

	return nil
}
