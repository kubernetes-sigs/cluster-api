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

// Package locking implements locking functionality.
package locking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const semaphoreInformationKey = "lock-information"

// ControlPlaneInitMutex uses a ConfigMap to synchronize cluster initialization.
type ControlPlaneInitMutex struct {
	client client.Client
}

// NewControlPlaneInitMutex returns a lock that can be held by a control plane node before init.
func NewControlPlaneInitMutex(client client.Client) *ControlPlaneInitMutex {
	return &ControlPlaneInitMutex{
		client: client,
	}
}

// Lock allows a control plane node to be the first and only node to run kubeadm init.
func (c *ControlPlaneInitMutex) Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool {
	sema := newSemaphore()
	cmName := configMapName(cluster.Name)
	log := ctrl.LoggerFrom(ctx, "ConfigMap", klog.KRef(cluster.Namespace, cmName))
	err := c.client.Get(ctx, client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, sema.ConfigMap)
	switch {
	case apierrors.IsNotFound(err):
		break
	case err != nil:
		log.Error(err, "Failed to acquire init lock")
		return false
	default: // Successfully found an existing config map.
		info, err := sema.information()
		if err != nil {
			log.Error(err, "Failed to get information about the existing init lock")
			return false
		}
		// The machine requesting the lock is the machine that created the lock, therefore the lock is acquired.
		if info.MachineName == machine.Name {
			return true
		}

		// If the machine that created the lock can not be found unlock the mutex.
		if err := c.client.Get(ctx, client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      info.MachineName,
		}, &clusterv1.Machine{}); err != nil {
			log.Error(err, "Failed to get machine holding init lock")
			if apierrors.IsNotFound(err) {
				c.Unlock(ctx, cluster)
			}
		}
		log.Info(fmt.Sprintf("Waiting for Machine %s to initialize", info.MachineName))
		return false
	}

	// Adds owner reference, namespace and name
	sema.setMetadata(cluster)
	// Adds the additional information
	if err := sema.setInformation(&information{MachineName: machine.Name}); err != nil {
		log.Error(err, "Failed to acquire init lock while setting semaphore information")
		return false
	}

	log.Info("Attempting to acquire the lock")
	err = c.client.Create(ctx, sema.ConfigMap)
	switch {
	case apierrors.IsAlreadyExists(err):
		log.Info("Cannot acquire the init lock. The init lock has been acquired by someone else")
		return false
	case err != nil:
		log.Error(err, "Error acquiring the init lock")
		return false
	default:
		return true
	}
}

// Unlock releases the lock.
func (c *ControlPlaneInitMutex) Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool {
	sema := newSemaphore()
	cmName := configMapName(cluster.Name)
	log := ctrl.LoggerFrom(ctx, "ConfigMap", klog.KRef(cluster.Namespace, cmName))
	err := c.client.Get(ctx, client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cmName,
	}, sema.ConfigMap)
	switch {
	case apierrors.IsNotFound(err):
		log.Info("Control plane init lock not found, it may have been released already")
		return true
	case err != nil:
		log.Error(err, "Error unlocking the control plane init lock")
		return false
	default:
		// Delete the config map semaphore if there is no error fetching it
		if err := c.client.Delete(ctx, sema.ConfigMap); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
			log.Error(err, "Error deleting the config map underlying the control plane init lock")
			return false
		}
		return true
	}
}

type information struct {
	MachineName string `json:"machineName"`
}

type semaphore struct {
	*corev1.ConfigMap
}

func newSemaphore() *semaphore {
	return &semaphore{&corev1.ConfigMap{}}
}

func configMapName(clusterName string) string {
	return fmt.Sprintf("%s-lock", clusterName)
}

func (s semaphore) information() (*information, error) {
	li := &information{}
	if err := json.Unmarshal([]byte(s.Data[semaphoreInformationKey]), li); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal semaphore information")
	}
	return li, nil
}

func (s semaphore) setInformation(information *information) error {
	b, err := json.Marshal(information)
	if err != nil {
		return errors.Wrap(err, "failed to marshal semaphore information")
	}
	s.Data = map[string]string{}
	s.Data[semaphoreInformationKey] = string(b)
	return nil
}

func (s *semaphore) setMetadata(cluster *clusterv1.Cluster) {
	s.ObjectMeta = metav1.ObjectMeta{
		Namespace: cluster.Namespace,
		Name:      configMapName(cluster.Name),
		Labels: map[string]string{
			clusterv1.ClusterNameLabel: cluster.Name,
		},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       cluster.Name,
				UID:        cluster.UID,
			},
		},
	}
}
