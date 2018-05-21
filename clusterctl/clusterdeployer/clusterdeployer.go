package clusterdeployer

import (
	"sigs.k8s.io/cluster-api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type ClusterDeployer struct {
}

func (*ClusterDeployer) Create(_ *clusterv1.Cluster, _ []*clusterv1.Machine) error {
	return errors.NotImplementedError
}
