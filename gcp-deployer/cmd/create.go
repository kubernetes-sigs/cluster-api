/*
Copyright 2017 The Kubernetes Authors.

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
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/gcp-deployer/deploy"
	"sigs.k8s.io/cluster-api/pkg/cert"
)

type CreateOptions struct {
	Cluster                  string
	Machine                  string
	MachineSetup             string
	CertificateAuthorityPath string
}

var co = &CreateOptions{}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			glog.Error("Please provide yaml file for cluster definition.")
			cmd.Help()
			os.Exit(1)
		}
		if co.Machine == "" {
			glog.Error("Please provide yaml file for machine definition.")
			cmd.Help()
			os.Exit(1)
		}
		if co.MachineSetup == "" {
			glog.Error("Please provide yaml file for machine setup configs.")
			cmd.Help()
			os.Exit(1)
		}
		if err := RunCreate(co); err != nil {
			glog.Exit(err)
		}
	},
}

func RunCreate(co *CreateOptions) error {
	cluster, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return err
	}

	machines, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return err
	}

	ca, err := loadCA()
	if err != nil {
		return err
	}

	d := deploy.NewDeployer(provider, kubeConfig, co.MachineSetup, ca)

	return d.CreateCluster(cluster, machines)
}

func init() {
	createCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "cluster yaml file")
	createCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "machine yaml file")
	createCmd.Flags().StringVarP(&co.MachineSetup, "machinesetup", "s", "machine_setup_configs.yaml", "machine setup configs yaml file")
	caHelpMessage := `optional path to a custom certificate authority to be used on a new cluster, path can be one of the following:
                                             1. directory: a path to a directory, the directory must contain two files named ca.crt and ca.key containing the certificate and private key respectively.
                                             2. certificate: a path to a certificate file, ${filename}.crt, the file must end with extension '.crt' and there must be a file named ${filename}.key in the same directory.
                                             3. key: a path to a key file, ${filename}.key, the file must end with extension '.key' and there must be a file named ${filename}.crt in the same directory.`
	createCmd.Flags().StringVarP(&co.CertificateAuthorityPath, "certificate-authority-path", "a", "", caHelpMessage)

	RootCmd.AddCommand(createCmd)
}

func loadCA() (*cert.CertificateAuthority, error) {
	if co.CertificateAuthorityPath == "" {
		return nil, nil
	}
	return cert.Load(co.CertificateAuthorityPath)
}
