// Copyright © 2017 Kris Nova <kris@nivenly.com>
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
//
//  _  ___
// | |/ / | ___  _ __   ___
// | ' /| |/ _ \| '_ \ / _ \
// | . \| | (_) | | | |  __/
// |_|\_\_|\___/|_| |_|\___|
//
// root.go is the cobra command for the primary klone command

package cmd

import (
	"fmt"
	"github.com/kris-nova/klone/pkg/auth"
	"github.com/kris-nova/klone/pkg/container"
	"github.com/kris-nova/klone/pkg/klone"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/spf13/cobra"
	"os"
)

var RootCmd = &cobra.Command{
	Use:   "klone",
	Short: "klone <query>",
	Long:  `klone provides easy functionality to begin working, running, and contributing to software repositories.`,
	Run:   runKlone,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

var containerOptions = &container.Options{
	//Command: []string{"sleep", "10"},
	Command: []string{"/bin/bash"},
}

func init() {
	RootCmd.Flags().StringVarP(&auth.OptPrivateKey, "identity-file", "i", "~/.ssh/id_rsa", "The private key to use for a git clone operation.")
	RootCmd.Flags().BoolVarP(&klone.RefreshCredentials, "refresh-credentials", "r", false, "Hard reset local credential cache")
	RootCmd.Flags().StringVarP(&containerOptions.Image, "container", "c", "", "Run the klone in a container, and use the image string defined")
	RootCmd.Flags().StringSliceVarP(&containerOptions.Command, "container-command", "x", []string{"/bin/bash"}, "The command to run in the container that we are kloning into.")
	local.PrintStartBanner()
	RootCmd.SetUsageTemplate(UsageTemplate)
	if len(os.Args) <= 1 {
		RootCmd.Help()
		os.Exit(0)
	}
}

func runKlone(cmd *cobra.Command, args []string) {
	local.SPutContent(local.Version, fmt.Sprintf("%s/.klone/version", local.Home()))
	query := args[0]
	if containerOptions.Image != "" {
		containerOptions.Query = query
		err := container.Run(containerOptions)
		if err != nil {
			local.PrintError(err)
			os.Exit(3)
		}
	} else {
		err := klone.Klone(query)
		if err != nil {
			local.PrintError(err)
			os.Exit(4)
		}
	}
}

const UsageTemplate = `Usage:{{if .Runnable}}
  {{if .HasAvailableFlags}}{{appendIfNotPresent .UseLine "<query> [flags]"}}{{else}}{{.UseLine}}{{end}}{{end}}{{if .HasAvailableSubCommands}}
  {{ .CommandPath}} [command]{{end}}{{if gt .Aliases 0}}

Aliases:
  {{.NameAndAliases}}
{{end}}{{if .HasExample}}

Examples:
{{ .Example }}{{end}}{{ if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{ if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{ if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsHelpCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{ if .HasAvailableSubCommands }}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
