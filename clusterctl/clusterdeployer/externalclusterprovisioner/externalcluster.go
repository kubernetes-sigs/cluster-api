package externalclusterprovisioner

import (
	"io/ioutil"
	"os"
	"fmt"
)

// Represents an actual external cluster being used, should not be able to
// actually delete or create, but can point to actual kubeconfig file.
type externalCluster struct {
	kubeconfigPath string
	kubeconfigFile string
}


func NewExternalCluster(kubeconfigPath string) (*externalCluster, error) {
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file at %s does not exist", kubeconfigPath)
	}

	return &externalCluster{kubeconfigPath:kubeconfigPath}, nil
}


func (e *externalCluster) Create() error {
	// noop
	return nil
}

func (e *externalCluster) Delete() error {
	// noop
	return nil
}

func (e *externalCluster) GetKubeconfig() (string, error) {

	if e.kubeconfigFile == "" {
		b, err := ioutil.ReadFile(e.kubeconfigPath)
		if err != nil {
			return "", err
		}

		e.kubeconfigFile = string(b)
	}


	return e.kubeconfigFile, nil
}