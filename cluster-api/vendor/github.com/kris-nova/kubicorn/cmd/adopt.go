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

	"github.com/spf13/cobra"
)

// AdoptCmd represents the adopt command
func AdoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt",
		Short: "Adopt a Kubernetes cluster into a Kubicorn state store",
		Long: `Use this command to audit and adopt a Kubernetes cluster into a Kubicorn state store.
	
	This command will query cloud resources and attempt to build a representation of the cluster in the Kubicorn API model.
	Once the cluster has been adopted, a user can manage and scale their Kubernetes cluster with Kubicorn.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("adopt called")
		},
	}
}
