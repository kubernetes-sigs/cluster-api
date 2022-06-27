/*
Copyright 2022 The Kubernetes Authors.

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
	"io/ioutil"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type exportOptions struct {
	coreProvider            string
	bootstrapProviders      []string
	controlPlaneProviders   []string
	infrastructureProviders []string
	targetNamespace         string
	skiptemplateProcess     bool
}

var exportOpts = &exportOptions{}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Exports the Cluster API manifests",
	Long:  "Exports the Cluster API manifests",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExport()
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportOpts.coreProvider, "core", "",
		"Core provider version (e.g. cluster-api:v0.3.0) to add to the management cluster. If unspecified, Cluster API's latest release is used.")
	exportCmd.Flags().StringSliceVarP(&exportOpts.infrastructureProviders, "infrastructure", "i", nil,
		"Infrastructure providers and versions (e.g. aws:v0.5.0) to add to the management cluster.")
	exportCmd.Flags().StringSliceVarP(&exportOpts.bootstrapProviders, "bootstrap", "b", nil,
		"Bootstrap providers and versions (e.g. kubeadm:v0.3.0) to add to the management cluster. If unspecified, Kubeadm bootstrap provider's latest release is used.")
	exportCmd.Flags().StringSliceVarP(&exportOpts.controlPlaneProviders, "control-plane", "c", nil,
		"Control plane providers and versions (e.g. kubeadm:v0.3.0) to add to the management cluster. If unspecified, the Kubeadm control plane provider's latest release is used.")
	exportCmd.Flags().StringVarP(&exportOpts.targetNamespace, "target-namespace", "n", "",
		"The target namespace where the providers should be deployed. If unspecified, the provider components' default namespace is used.")
	exportCmd.Flags().BoolVar(&exportOpts.skiptemplateProcess, "skip-template-process", false,
		"Should the templating process (including variable substitution) be skipped")

	alphaCmd.AddCommand(exportCmd)
}

func runExport() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	options := client.ExportOptions{
		CoreProvider:            exportOpts.coreProvider,
		BootstrapProviders:      exportOpts.bootstrapProviders,
		ControlPlaneProviders:   exportOpts.controlPlaneProviders,
		InfrastructureProviders: exportOpts.infrastructureProviders,
		TargetNamespace:         exportOpts.targetNamespace,
	}

	data, err := c.Export(options)
	if err != nil {
		return err
	}

	ioutil.WriteFile("export.yaml", data, 0644)

	return nil
}
