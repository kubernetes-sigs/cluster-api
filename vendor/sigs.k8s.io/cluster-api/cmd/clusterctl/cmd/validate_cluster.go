/*
Copyright 2018 The Kubernetes Authors.

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
	"sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/spf13/cobra"

	tcmd "k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type ValidateClusterOptions struct {
	KubeconfigOverrides tcmd.ConfigOverrides
}

var vco = &ValidateClusterOptions{}

var validateClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Validate a cluster created by cluster API.",
	Long:  `Validate a cluster created by cluster API.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunValidateCluster(); err != nil {
			os.Stdout.Sync()
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// BindContextFlags will bind the flags cluster, namespace, and user
	tcmd.BindContextFlags(&vco.KubeconfigOverrides.Context, validateClusterCmd.Flags(), tcmd.RecommendedContextOverrideFlags(""))
	validateCmd.AddCommand(validateClusterCmd)
}

func RunValidateCluster() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to create client configuration: %v", err)
	}
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		return fmt.Errorf("failed to create manager: %v", err)
	}
	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add APIs to manager: %v", err)
	}

	c, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()})
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}
	if err = validation.ValidateClusterAPIObjects(os.Stdout, c, vco.KubeconfigOverrides.Context.Cluster, vco.KubeconfigOverrides.Context.Namespace); err != nil {
		return err
	}

	// TODO(wangzhen127): Also validate the cluster in addition to the cluster API objects. https://github.com/kubernetes-sigs/cluster-api/issues/168
	return nil
}
