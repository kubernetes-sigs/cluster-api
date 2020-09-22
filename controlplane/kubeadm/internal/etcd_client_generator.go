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
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
)

// etcdClientGenerator generates etcd clients that connect to specific etcd members on particular control plane nodes.
type etcdClientGenerator struct {
	restConfig *rest.Config
	tlsConfig  *tls.Config
}

func (c *etcdClientGenerator) forNodes(ctx context.Context, nodes []corev1.Node) (*etcd.Client, error) {
	endpoints := make([]string, len(nodes))
	for i, node := range nodes {
		endpoints[i] = staticPodName(EtcdPodNamePrefix, node.Name)
	}

	p := proxy.Proxy{
		Kind:       "pods",
		Namespace:  metav1.NamespaceSystem,
		KubeConfig: c.restConfig,
		TLSConfig:  c.tlsConfig,
		Port:       2379,
	}
	return etcd.NewClient(ctx, endpoints, p, c.tlsConfig)
}

// forLeader takes a list of nodes and returns a client to the leader node
func (c *etcdClientGenerator) forLeader(ctx context.Context, nodes []corev1.Node) (*etcd.Client, error) {
	var errs []error

	for _, node := range nodes {
		client, err := c.forNodes(ctx, []corev1.Node{node})
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
			if member.Name == node.Name && member.ID == client.LeaderID {
				return c.forNodes(ctx, []corev1.Node{node})
			}
		}
	}

	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to the etcd leader")
}
