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
