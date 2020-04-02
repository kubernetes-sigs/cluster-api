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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
)

// etcdClientGenerator generates etcd clients that connect to specific etcd members on particular control plane nodes.
type etcdClientGenerator struct {
	restConfig *rest.Config
	tlsConfig  *tls.Config
}

func (c *etcdClientGenerator) forNode(ctx context.Context, name string) (*etcd.Client, error) {
	// This does not support external etcd.
	p := proxy.Proxy{
		Kind:         "pods",
		Namespace:    metav1.NamespaceSystem, // TODO, can etcd ever run in a different namespace?
		ResourceName: staticPodName("etcd", name),
		KubeConfig:   c.restConfig,
		TLSConfig:    c.tlsConfig,
		Port:         2379, // TODO: the pod doesn't expose a port. Is this a problem?
	}
	dialer, err := proxy.NewDialer(p)
	if err != nil {
		return nil, err
	}
	etcdclient, err := etcd.NewEtcdClient("127.0.0.1", dialer.DialContextWithAddr, c.tlsConfig)
	if err != nil {
		return nil, err
	}
	customClient, err := etcd.NewClientWithEtcd(ctx, etcdclient)
	if err != nil {
		return nil, err
	}
	return customClient, nil
}

// forLeader takes a list of nodes and returns a client to the leader node
func (c *etcdClientGenerator) forLeader(ctx context.Context, nodes *corev1.NodeList) (*etcd.Client, error) {
	var errs []error

	for _, node := range nodes.Items {
		client, err := c.forNode(ctx, node.Name)
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
			if member.ID == client.LeaderID {
				return c.forNode(ctx, member.Name)
			}
		}
	}

	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to the etcd leader")
}
