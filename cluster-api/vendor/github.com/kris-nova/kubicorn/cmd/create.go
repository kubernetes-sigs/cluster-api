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

package cmd

import (
	"fmt"
	"math"
	"os"
	"os/user"
	"strings"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/namer"
	"github.com/kris-nova/kubicorn/profiles/amazon"
	"github.com/kris-nova/kubicorn/profiles/azure"
	"github.com/kris-nova/kubicorn/profiles/digitalocean"
	"github.com/kris-nova/kubicorn/profiles/googlecompute"
	"github.com/kris-nova/kubicorn/state"
	"github.com/kris-nova/kubicorn/state/fs"
	"github.com/kris-nova/kubicorn/state/jsonfs"
	"github.com/spf13/cobra"
	"github.com/yuroyoro/swalker"
)

type CreateOptions struct {
	Options
	Profile string
}

var co = &CreateOptions{}

// CreateCmd represents create command
func CreateCmd() *cobra.Command {
	var createCmd = &cobra.Command{
		Use:   "create [NAME] [-p|--profile PROFILENAME] [-c|--cloudid CLOUDID]",
		Short: "Create a Kubicorn API model from a profile",
		Long: `Use this command to create a Kubicorn API model in a defined state store.
	
	This command will create a cluster API model as a YAML manifest in a state store.
	Once the API model has been created, a user can optionally change the model to their liking.
	After a model is defined and configured properly, the user can then apply the model.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				co.Name = strEnvDef("KUBICORN_NAME", namer.RandomName())
			} else if len(args) > 1 {
				logger.Critical("Too many arguments.")
				os.Exit(1)
			} else {
				co.Name = args[0]
			}

			err := RunCreate(co)
			if err != nil {
				logger.Critical(err.Error())
				os.Exit(1)
			}

		},
	}

	createCmd.Flags().StringVarP(&co.StateStore, "state-store", "s", strEnvDef("KUBICORN_STATE_STORE", "fs"), "The state store type to use for the cluster")
	createCmd.Flags().StringVarP(&co.StateStorePath, "state-store-path", "S", strEnvDef("KUBICORN_STATE_STORE_PATH", "./_state"), "The state store path to use")
	createCmd.Flags().StringVarP(&co.Profile, "profile", "p", strEnvDef("KUBICORN_PROFILE", "azure"), "The cluster profile to use")
	createCmd.Flags().StringVarP(&co.CloudId, "cloudid", "c", strEnvDef("KUBICORN_CLOUDID", ""), "The cloud id")
	createCmd.Flags().StringVarP(&co.Set, "set", "e", strEnvDef("KUBICORN_SET", ""), "set cluster setting")

	flagApplyAnnotations(createCmd, "profile", "__kubicorn_parse_profiles")
	flagApplyAnnotations(createCmd, "cloudid", "__kubicorn_parse_cloudid")

	return createCmd
}

// TODO(xmudrii): revisit this part
/*func init() {
	RootCmd.SetUsageTemplate(usageTemplate)
	RootCmd.AddCommand(createCmd)
}*/

type profileFunc func(name string) *cluster.Cluster

type profileMap struct {
	profileFunc profileFunc
	description string
}

var profileMapIndexed = map[string]profileMap{
	"azure": {
		profileFunc: azure.NewUbuntuCluster,
		description: "Ubuntu on Azure",
	},
	"azure-ubuntu": {
		profileFunc: azure.NewUbuntuCluster,
		description: "Ubuntu on Azure",
	},
	"amazon": {
		profileFunc: amazon.NewUbuntuCluster,
		description: "Ubuntu on Amazon",
	},
	"aws": {
		profileFunc: amazon.NewUbuntuCluster,
		description: "Ubuntu on Amazon",
	},
	"do": {
		profileFunc: digitalocean.NewUbuntuCluster,
		description: "Ubuntu on DigitalOcean",
	},
	"google": {
		profileFunc: googlecompute.NewUbuntuCluster,
		description: "Ubuntu on Google Compute",
	},
	"digitalocean": {
		profileFunc: digitalocean.NewUbuntuCluster,
		description: "Ubuntu on DigitalOcean",
	},
	"do-ubuntu": {
		profileFunc: digitalocean.NewUbuntuCluster,
		description: "Ubuntu on DigitalOcean",
	},
	"aws-ubuntu": {
		profileFunc: amazon.NewUbuntuCluster,
		description: "Ubuntu on Amazon",
	},
	"do-centos": {
		profileFunc: digitalocean.NewCentosCluster,
		description: "CentOS on DigitalOcean",
	},
	"aws-centos": {
		profileFunc: amazon.NewCentosCluster,
		description: "CentOS on Amazon",
	},
}

// RunCreate is the starting point when a user runs the create command.
func RunCreate(options *CreateOptions) error {

	// Create our cluster resource
	name := options.Name
	var newCluster *cluster.Cluster
	if _, ok := profileMapIndexed[options.Profile]; ok {
		newCluster = profileMapIndexed[options.Profile].profileFunc(name)
	} else {
		return fmt.Errorf("Invalid profile [%s]", options.Profile)
	}

	if options.Set != "" {
		sets := strings.Split(options.Set, ",")
		for _, set := range sets {
			parts := strings.SplitN(set, "=", 2)
			if len(parts) == 1 {
				continue
			}
			err := swalker.Write(strings.Title(parts[0]), newCluster, parts[1])
			if err != nil {
				println(err)
			}
		}
	}

	if newCluster.Cloud == cluster.CloudGoogle && options.CloudId == "" {
		return fmt.Errorf("CloudID is required for google cloud. Please set it to your project ID")
	}
	newCluster.CloudId = options.CloudId

	// Expand state store path
	// Todo (@kris-nova) please pull this into a filepath package or something
	options.StateStorePath = expandPath(options.StateStorePath)

	// Register state store
	var stateStore state.ClusterStorer
	switch options.StateStore {
	case "fs":
		logger.Info("Selected [fs] state store")
		stateStore = fs.NewFileSystemStore(&fs.FileSystemStoreOptions{
			BasePath:    options.StateStorePath,
			ClusterName: name,
		})
	case "jsonfs":
		logger.Info("Selected [jsonfs] state store")
		stateStore = jsonfs.NewJSONFileSystemStore(&jsonfs.JSONFileSystemStoreOptions{
			BasePath:    options.StateStorePath,
			ClusterName: name,
		})
	}

	// Check if state store exists
	if stateStore.Exists() {
		return fmt.Errorf("State store [%s] exists, will not overwrite. Delete existing profile [%s] and retry", name, options.StateStorePath+"/"+name)
	}

	// Init new state store with the cluster resource
	err := stateStore.Commit(newCluster)
	if err != nil {
		return fmt.Errorf("Unable to init state store: %v", err)
	}

	logger.Always("The state [%s/%s/cluster.yaml] has been created. You can edit the file, then run `kubicorn apply %s`", options.StateStorePath, name, name)
	return nil
}

func expandPath(path string) string {
	if path == "." {
		wd, err := os.Getwd()
		if err != nil {
			logger.Critical("Unable to get current working directory: %v", err)
			return ""
		}
		path = wd
	}
	if path == "~" {
		homeVar := os.Getenv("HOME")
		if homeVar == "" {
			homeUser, err := user.Current()
			if err != nil {
				logger.Critical("Unable to use user.Current() for user. Maybe a cross compile issue: %v", err)
				return ""
			}
			path = homeUser.HomeDir
		}
	}
	return path
}

var (
	p = func() string {
		str := ""
		spaces := ""
		maxLen := 0
		for shorthand := range profileMapIndexed {
			l := len(shorthand)
			if l > maxLen {
				maxLen = l
			}
		}
		for shorthand, pmap := range profileMapIndexed {
			spaces = ""
			k := math.Abs(float64(maxLen) - float64(len(shorthand)) + 3)
			for i := 0; i < int(k); i++ {
				spaces = fmt.Sprintf("%s%s", spaces, " ")
			}
			str = fmt.Sprintf("%s   %s%s %s\n", str, shorthand, spaces, pmap.description)
		}
		return str
	}()
	usageTemplate = fmt.Sprintf(`Usage:{{if .Runnable}}
  {{if .HasAvailableFlags}}{{appendIfNotPresent .UseLine "[flags]"}}{{else}}{{.UseLine}}{{end}}{{end}}{{if .HasAvailableSubCommands}}
  {{ .CommandPath}} [command]{{end}}

Profiles:
%s{{if gt .Aliases 0}}
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
`, p)
)
