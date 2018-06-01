package deploy

import (
	"sigs.k8s.io/cluster-api/pkg/controller/cluster"
)

// Provider-specific cluster logic the deployer needs.
type clusterDeployer interface {
	cluster.Actuator
}
