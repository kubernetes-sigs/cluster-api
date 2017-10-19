// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var commandFlagSuggestions = make(map[string][]prompt.Suggest)
var commandMap = make(map[string][]prompt.Suggest)
var promptSuggestions = []prompt.Suggest{}

// PromptCmd represents the kubicorn interactive prompt.
func PromptCmd() *cobra.Command {
	var promptCmd = &cobra.Command{
		Use:   "prompt",
		Short: "Open a prompt with auto-completion (non-Windows)",
		Long: `Use this command to use the Kubicron API via a shell prompt.
	
	This command will open a prompt using go-prompt (with auto-completion) to
	allow you to run commands interactively from the shell.
	Currently this doesn't work on Windows systems`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				logger.Critical("Too many arguments.")
				os.Exit(1)
			}
			err := RunPrompt()
			if err != nil {
				logger.Critical(err.Error())
				os.Exit(1)
			}
		},
	}

	initializePrompt()

	return promptCmd
}

func RunPrompt() error {
	fmt.Println("Please input your kubicorn commands.")
	defer fmt.Println("Bye!")

	p := prompt.New(
		executor,
		completer,
		prompt.OptionTitle("kubicorn: interactive kubicorn client"),
		prompt.OptionPrefix(">> "),
	)

	p.Run()
	return nil
}

func executor(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	} else if s == "quit" || s == "exit" {
		fmt.Println("Bye!")
		os.Exit(0)
		return
	}

	commandToExecute := exec.Command("/bin/sh", "-c", "kubicorn "+s)
	commandToExecute.Stdin = os.Stdin
	commandToExecute.Stdout = os.Stdout
	commandToExecute.Stderr = os.Stderr
	if err := commandToExecute.Run(); err != nil {
		fmt.Printf("Got error: %s\n", err.Error())
	}
	return
}

func completer(d prompt.Document) []prompt.Suggest {
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	args := strings.Split(d.TextBeforeCursor(), " ")
	w := d.GetWordBeforeCursor()

	// If PIPE is in text before the cursor, returns empty suggestions.
	for i := range args {
		if args[i] == "|" {
			return []prompt.Suggest{}
		}
	}

	// If word before the cursor starts with "-", returns CLI flag options.
	if strings.HasPrefix(w, "-") {
		return optionCompleter(args)
	}

	return argumentsCompleter(excludeOptions(args))
}

func argumentsCompleter(args []string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(promptSuggestions, args[0], true)
	}
	first := args[0]
	return prompt.FilterHasPrefix(promptSuggestions, first, true)
}

func excludeOptions(args []string) []string {
	ret := make([]string, 0, len(args))
	for i := range args {
		if !strings.HasPrefix(args[i], "-") {
			ret = append(ret, args[i])
		}
	}
	return ret
}

func initializePrompt() {
	// Get the commands from Cobra to build the prompt suggestions
	for _, command := range RootCmd.Commands() {
		// Get the flags for each of the commands in order to enable option completion
		var flagSuggestions = []prompt.Suggest{}
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			flagSuggestions = append(flagSuggestions, prompt.Suggest{
				//Adding the -- to allow auto-complete to work on the flags flawlessly
				Text:        "--" + flag.Name,
				Description: flag.Usage,
			})
		})
		commandFlagSuggestions[command.Name()] = flagSuggestions

		var promptSuggestion = prompt.Suggest{
			Text:        command.Name(),
			Description: command.Short,
		}
		promptSuggestions = append(promptSuggestions, promptSuggestion)
	}
}

func optionCompleter(args []string) []prompt.Suggest {
	l := len(args)
	var flagSuggestions []prompt.Suggest

	command := args[0]
	flagSuggestions = commandFlagSuggestions[command]

	return prompt.FilterContains(flagSuggestions, strings.TrimLeft(args[l-1], "-"), true)
}
