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
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
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

type clientCreator func(ctx context.Context, endpoint string) (*etcd.Client, error)

// NewEtcdClientGenerator returns a new etcdClientGenerator instance.
func NewEtcdClientGenerator(restConfig *rest.Config, tlsConfig *tls.Config, etcdDialTimeout, etcdCallTimeout time.Duration, etcdLogger *zap.Logger) *EtcdClientGenerator {
	ecg := &EtcdClientGenerator{restConfig: restConfig, tlsConfig: tlsConfig}

	ecg.createClient = func(ctx context.Context, endpoint string) (*etcd.Client, error) {
		p := proxy.Proxy{
			Kind:       "pods",
			Namespace:  metav1.NamespaceSystem,
			KubeConfig: ecg.restConfig,
			Port:       2379,
		}
		return etcd.NewClient(ctx, etcd.ClientConfiguration{
			Endpoint:    endpoint,
			Proxy:       p,
			TLSConfig:   tlsConfig,
			DialTimeout: etcdDialTimeout,
			CallTimeout: etcdCallTimeout,
			Logger:      etcdLogger,
		})
	}

	return ecg
}

// forFirstAvailableNode takes a list of nodes and returns a client for the first one that connects.
func (c *EtcdClientGenerator) forFirstAvailableNode(ctx context.Context, nodeNames []string) (*etcd.Client, error) {
	// This is an additional safeguard for avoiding this func to return nil, nil.
	if len(nodeNames) == 0 {
		return nil, errors.New("invalid argument: forFirstAvailableNode can't be called with an empty list of nodes")
	}

	// Loop through the existing control plane nodes.
	var errs []error
	for _, name := range nodeNames {
		endpoint := staticPodName("etcd", name)
		client, err := c.createClient(ctx, endpoint)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return client, nil
	}
	return nil, errors.Wrapf(kerrors.NewAggregate(errs), "could not establish a connection to etcd members hosted on %s", strings.Join(nodeNames, ","))
}
