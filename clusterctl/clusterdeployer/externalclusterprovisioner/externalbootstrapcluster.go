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

package externalclusterprovisioner

import (
	"io/ioutil"
	"os"
	"fmt"
)

// Represents an actual external cluster being used for bootstrapping, should not be able to
// actually delete or create, but can point to actual kubeconfig file.
type ExternalBootstrapCluster struct {
	kubeconfigPath string
	kubeconfigFile string
}

// NewExternalCluster creates a new external k8s bootstrap cluster object
// We should clean up any lingering resources when clusterctl is complete.
// TODO https://github.com/kubernetes-sigs/cluster-api/issues/448
func NewExternalCluster(kubeconfigPath string) (*ExternalBootstrapCluster, error) {
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file at %s does not exist", kubeconfigPath)
	}

	return &ExternalBootstrapCluster{kubeconfigPath:kubeconfigPath}, nil
}

// Create implements clusterdeployer.ClusterProvisioner interface
func (e *ExternalBootstrapCluster) Create() error {
	// noop
	return nil
}
// Delete implements clusterdeployer.ClusterProvisioner interface
func (e *ExternalBootstrapCluster) Delete() error {
	// noop
	return nil
}

// GetKubeconfig implements clusterdeployer.ClusterProvisioner interface
func (e *ExternalBootstrapCluster) GetKubeconfig() (string, error) {

	if e.kubeconfigFile == "" {
		b, err := ioutil.ReadFile(e.kubeconfigPath)
		if err != nil {
			return "", err
		}

		e.kubeconfigFile = string(b)
	}


	return e.kubeconfigFile, nil
}
