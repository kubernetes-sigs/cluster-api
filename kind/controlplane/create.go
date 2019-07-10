package controlplane

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/cri"
)

func CreateKindCluster(image, clusterName string) error {
	lb, err := actions.SetUpLoadBalancer(clusterName)
	if err != nil {
		return errors.Wrap(err, "failed to create load balancer")
	}

	lbipv4, _, err := lb.IP()
	if err != nil {
		return errors.Wrap(err, "failed to get ELB IP")
	}

	cpMounts := []cri.Mount{
		{
			ContainerPath: "/var/run/docker.sock",
			HostPath:      "/var/run/docker.sock",
		},
	}

	cp, err := actions.CreateControlPlane(clusterName, fmt.Sprintf("%s-control-plane", clusterName), lbipv4, "v1.14.2", cpMounts)
	if err != nil {
		return errors.Wrap(err, "couldn't create control plane")
	}

	if !nodes.WaitForReady(cp, time.Now().Add(5*time.Minute)) {
		return errors.New("control plane was not ready in 5 minutes")
	}

	return nil
}
