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
	"crypto/x509"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	ctrlclient.Reader

	GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...machinefilters.Func) (FilterableMachineCollection, error)
	GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey) (WorkloadCluster, error)
}

// Management holds operations on the management cluster.
type Management struct {
	Client ctrlclient.Reader
}

// RemoteClusterConnectionError represents a failure to connect to a remote cluster
type RemoteClusterConnectionError struct {
	Name string
	Err  error
}

func (e *RemoteClusterConnectionError) Error() string { return e.Name + ": " + e.Err.Error() }
func (e *RemoteClusterConnectionError) Unwrap() error { return e.Err }

// Get implements ctrlclient.Reader
func (m *Management) Get(ctx context.Context, key ctrlclient.ObjectKey, obj runtime.Object) error {
	return m.Client.Get(ctx, key, obj)
}

// List implements ctrlclient.Reader
func (m *Management) List(ctx context.Context, list runtime.Object, opts ...ctrlclient.ListOption) error {
	return m.Client.List(ctx, list, opts...)
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...machinefilters.Func) (FilterableMachineCollection, error) {
	selector := map[string]string{
		clusterv1.ClusterLabelName: cluster.Name,
	}
	ml := &clusterv1.MachineList{}
	if err := m.Client.List(ctx, ml, client.InNamespace(cluster.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}

	machines := NewFilterableMachineCollectionFromMachineList(ml)
	return machines.Filter(filters...), nil
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey) (WorkloadCluster, error) {
	// TODO(chuckha): Inject this dependency.
	// TODO(chuckha): memoize this function. The workload client only exists as long as a reconciliation loop.
	restConfig, err := remote.RESTConfig(ctx, m.Client, clusterKey)
	if err != nil {
		return nil, err
	}
	restConfig.Timeout = 30 * time.Second

	c, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}

	// Retrieves the etcd CA key Pair
	crtData, keyData, err := m.getEtcdCAKeyPair(ctx, clusterKey)
	if err != nil {
		return nil, err
	}

	// If the CA key is defined, the cluster is using a managed etcd, and so we can generate a new
	// etcd client certificate for the controllers.
	// Otherwise the cluster is using an external etcd; in this case the only option to connect to etcd is to re-use
	// the apiserver-etcd-client certificate.
	// TODO: consider if we can detect if we are using external etcd in a more explicit way (e.g. looking at the config instead of deriving from the existing certificates)
	var clientCert tls.Certificate
	if keyData != nil {
		clientCert, err = generateClientCert(crtData, keyData)
		if err != nil {
			return nil, err
		}
	} else {
		clientCert, err = m.getApiServerEtcdClientCert(ctx, clusterKey)
		if err != nil {
			return nil, err
		}
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(crtData)
	cfg := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
	}
	cfg.InsecureSkipVerify = true
	return &Workload{
		Client:          c,
		CoreDNSMigrator: &CoreDNSMigrator{},
		etcdClientGenerator: &etcdClientGenerator{
			restConfig: restConfig,
			tlsConfig:  cfg,
		},
	}, nil
}

func (m *Management) getEtcdCAKeyPair(ctx context.Context, clusterKey ctrlclient.ObjectKey) ([]byte, []byte, error) {
	etcdCASecret := &corev1.Secret{}
	etcdCAObjectKey := ctrlclient.ObjectKey{
		Namespace: clusterKey.Namespace,
		Name:      fmt.Sprintf("%s-etcd", clusterKey.Name),
	}
	if err := m.Client.Get(ctx, etcdCAObjectKey, etcdCASecret); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", etcdCAObjectKey.Namespace, etcdCAObjectKey.Name)
	}
	crtData, ok := etcdCASecret.Data[secret.TLSCrtDataName]
	if !ok {
		return nil, nil, errors.Errorf("etcd tls crt does not exist for cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
	}
	keyData := etcdCASecret.Data[secret.TLSKeyDataName]
	return crtData, keyData, nil
}

func (m *Management) getApiServerEtcdClientCert(ctx context.Context, clusterKey ctrlclient.ObjectKey) (tls.Certificate, error) {
	apiServerEtcdClientCertificateSecret := &corev1.Secret{}
	apiServerEtcdClientCertificateObjectKey := ctrlclient.ObjectKey{
		Namespace: clusterKey.Namespace,
		Name:      fmt.Sprintf("%s-apiserver-etcd-client", clusterKey.Name),
	}
	if err := m.Client.Get(ctx, apiServerEtcdClientCertificateObjectKey, apiServerEtcdClientCertificateSecret); err != nil {
		return tls.Certificate{}, errors.Wrapf(err, "failed to get secret; etcd apiserver-etcd-client %s/%s", apiServerEtcdClientCertificateObjectKey.Namespace, apiServerEtcdClientCertificateObjectKey.Name)
	}
	crtData, ok := apiServerEtcdClientCertificateSecret.Data[secret.TLSCrtDataName]
	if !ok {
		return tls.Certificate{}, errors.Errorf("etcd tls crt does not exist for cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
	}
	keyData, ok := apiServerEtcdClientCertificateSecret.Data[secret.TLSKeyDataName]
	if !ok {
		return tls.Certificate{}, errors.Errorf("etcd tls key does not exist for cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
	}
	return tls.X509KeyPair(crtData, keyData)
}
