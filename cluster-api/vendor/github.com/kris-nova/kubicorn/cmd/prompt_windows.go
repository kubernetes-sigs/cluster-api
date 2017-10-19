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

// +build windows

package cmd

import (
	"os"

	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/spf13/cobra"
)

// PromptCmd represents the kubicorn interactive prompt.
func PromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Open a prompt with auto-completion (Windows)",
		Long: `Use this command to use the Kubicron API via a shell prompt.
	
	This command will open a prompt using go-prompt (with auto-completion) to
	allow you to run commands interactively from the shell.
	Currently this doesn't work on Windows systems.`,
		Run: func(cmd *cobra.Command, args []string) {
			logger.Critical("Sorry. kubicorn prompt is not available for Windows machines")
			os.Exit(1)
		},
	}
}
