/*
Copyright 2019 The Kubernetes Authors.

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

package controlplane

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/cri"
)

// CreateKindCluster sets up a  KIND cluster and turns it into a CAPD control plane
func CreateKindCluster(clusterName string) error {
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
		{

			ContainerPath: "/var/lib/docker",
			HostPath:      "/var/lib/docker",
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
