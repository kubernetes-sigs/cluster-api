/*
Copyright 2020 The Kubernetes Authors.

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

package internal

import (
	"context"
	"crypto/tls"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
)

// EtcdClientGenerator generates etcd clients that connect to specific etcd members on particular control plane nodes.
type EtcdClientGenerator struct {
	restConfig   *rest.Config
	tlsConfig    *tls.Config
	createClient clientCreator
}

type clientCreator func(ctx context.Context, endpoints []string) (*etcd.Client, error)

// NewEtcdClientGenerator returns a new etcdClientGenerator instance.
func NewEtcdClientGenerator(restConfig *rest.Config, tlsConfig *tls.Config) *EtcdClientGenerator {
	ecg := &EtcdClientGenerator{restConfig: restConfig, tlsConfig: tlsConfig}

	ecg.createClient = func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
		p := proxy.Proxy{
			Kind:       "pods",
			Namespace:  metav1.NamespaceSystem,
			KubeConfig: ecg.restConfig,
			TLSConfig:  ecg.tlsConfig,
			Port:       2379,
		}
		return etcd.NewClient(ctx, endpoints, p, ecg.tlsConfig)
	}

	return ecg
}

// forFirstAvailableNode takes a list of nodes and returns a client for the first one that connects.
func (c *EtcdClientGenerator) forFirstAvailableNode(ctx context.Context, nodeNames []string) (*etcd.Client, error) {
	var errs []error
	for _, name := range nodeNames {
		endpoints := []string{staticPodName("etcd", name)}
		client, err := c.createClient(ctx, endpoints)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return client, nil
	}
	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to any etcd node")
}

// forLeader takes a list of nodes and returns a client to the leader node.
func (c *EtcdClientGenerator) forLeader(ctx context.Context, nodeNames []string) (*etcd.Client, error) {
	var errs []error

	for _, nodeName := range nodeNames {
		client, err := c.forFirstAvailableNode(ctx, []string{nodeName})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		defer client.Close()
		members, err := client.Members(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, member := range members {
			if member.Name == nodeName && member.ID == client.LeaderID {
				return c.forFirstAvailableNode(ctx, []string{nodeName})
			}
		}
	}

	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to the etcd leader")
}
