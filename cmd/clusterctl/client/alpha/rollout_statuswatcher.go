/*
Copyright 2022 The Kubernetes Authors.

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

package alpha

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// StatusWatcher provides an interface for resources that have rollout status.
type StatusWatcher interface {
	Status(proxy cluster.Proxy, name, namespace string) (string, bool, error)
}

// KubeadmControlPlaneStatusWatcher implements the StatusViewer interface.
type KubeadmControlPlaneStatusWatcher struct{}

// MachineDeploymentStatusWatcher implements the StatusViewer interface.
type MachineDeploymentStatusWatcher struct{}

// ObjectStatusWatcher will issue a view on the specified cluster-api resource.
func (r *rollout) ObjectStatusWatcher(proxy cluster.Proxy, ref corev1.ObjectReference) error {
	log := ctrl.LoggerFrom(ctx)
	var statusViewer StatusWatcher
	switch ref.Kind {
	case KubeadmControlPlane:
		statusViewer = &KubeadmControlPlaneStatusWatcher{}
	case MachineDeployment:
		statusViewer = &MachineDeploymentStatusWatcher{}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validResourceTypes)
	}

	oldStatus := ""
	timeout := 600 * time.Second
	lastChange := time.Now()
	for {
		status, done, err := statusViewer.Status(proxy, ref.Name, ref.Namespace)
		if err != nil {
			log.Error(err, "failed to get resource status")
			time.Sleep(2 * time.Second)
			continue
		}
		if status != oldStatus {
			fmt.Fprintf(os.Stdout, "%s", status)
			oldStatus = status
			lastChange = time.Now()
		}
		if done || time.Since(lastChange) > timeout {
			break
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

// Status returns a status describing kubeadmcontrolplane status, and a bool value indicating if the status is considered done.
func (s *KubeadmControlPlaneStatusWatcher) Status(proxy cluster.Proxy, name, namespace string) (string, bool, error) {
	newKcp, err := getKubeadmControlPlane(proxy, name, namespace)
	if err != nil || newKcp == nil {
		return "", false, errors.Wrapf(err, "failed to get kubeadmcontrolplane/%v", name)
	}
	if newKcp.Spec.Replicas != nil && newKcp.Status.UpdatedReplicas < *newKcp.Spec.Replicas {
		return fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d out of %d new replicas have been updated...\n", newKcp.Name, newKcp.Status.UpdatedReplicas, *newKcp.Spec.Replicas), false, nil
	}
	if newKcp.Status.Replicas > newKcp.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d old replicas are pending termination...\n", newKcp.Name, newKcp.Status.Replicas-newKcp.Status.UpdatedReplicas), false, nil
	}
	if newKcp.Status.ReadyReplicas < newKcp.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for kubeadmcontrolplane %q rollout to finish: %d of %d updated replicas are ready...\n", newKcp.Name, newKcp.Status.ReadyReplicas, newKcp.Status.UpdatedReplicas), false, nil
	}
	return fmt.Sprintf("kubeadmcontrolplane %q successfully rolled out\n", newKcp.Name), true, nil
}

// Status returns a status describing machinedeployment status, and a bool value indicating if the status is considered done.
func (s *MachineDeploymentStatusWatcher) Status(proxy cluster.Proxy, name, namespace string) (string, bool, error) {
	newMD, err := getMachineDeployment(proxy, name, namespace)
	if err != nil || newMD == nil {
		return "", false, errors.Wrapf(err, "failed to get machinedeployment/%v", name)
	}
	if newMD.Spec.Replicas != nil && newMD.Status.UpdatedReplicas < *newMD.Spec.Replicas {
		return fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d out of %d new replicas have been updated...\n", newMD.Name, newMD.Status.UpdatedReplicas, *newMD.Spec.Replicas), false, nil
	}
	if newMD.Status.Replicas > newMD.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d old replicas are pending termination...\n", newMD.Name, newMD.Status.Replicas-newMD.Status.UpdatedReplicas), false, nil
	}
	if newMD.Status.AvailableReplicas < newMD.Status.UpdatedReplicas {
		return fmt.Sprintf("Waiting for machinedeployment %q rollout to finish: %d of %d updated replicas are available...\n", newMD.Name, newMD.Status.AvailableReplicas, newMD.Status.UpdatedReplicas), false, nil
	}
	return fmt.Sprintf("machinedeployment %q successfully rolled out\n", newMD.Name), true, nil
}
