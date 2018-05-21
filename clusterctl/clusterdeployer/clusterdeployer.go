package clusterdeployer

import (
	"fmt"

	"sigs.k8s.io/cluster-api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// Can provision a kubernetes cluster
type ClusterProvisioner interface {
	Create() error
	Delete() error
	GetKubeconfig() (string, error)
}

type ClusterDeployer struct {
	externalProvisioner    ClusterProvisioner
	cleanupExternalCluster bool
}

func New(externalProvisioner ClusterProvisioner, cleanupExternalCluster bool) *ClusterDeployer {
	return &ClusterDeployer{
		externalProvisioner:    externalProvisioner,
		cleanupExternalCluster: cleanupExternalCluster,
	}
}

// Creates the a cluster from the provided cluster definition and machine list.
func (d *ClusterDeployer) Create(_ *clusterv1.Cluster, _ []*clusterv1.Machine) error {
	if err := d.externalProvisioner.Create(); err != nil {
		return fmt.Errorf("could not create external control plane: %v", err)
	}
	if d.cleanupExternalCluster {
		defer d.externalProvisioner.Delete()
	}
	return errors.NotImplementedError // Not fully functional yet.
}
