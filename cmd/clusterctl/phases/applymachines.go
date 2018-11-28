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

package phases

import (
	"fmt"

	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func ApplyMachines(client clusterclient.Client, namespace string, machines []*clusterv1.Machine) error {
	if namespace == "" {
		namespace = client.GetContextNamespace()
	}

	err := client.EnsureNamespace(namespace)
	if err != nil {
		return fmt.Errorf("unable to ensure namespace %q: %v", namespace, err)
	}

	klog.Infof("Creating machines in namespace %q", namespace)
	if err := client.CreateMachineObjects(machines, namespace); err != nil {
		return err
	}

	return nil
}
