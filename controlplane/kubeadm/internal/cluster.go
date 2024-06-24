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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/secret"
)

const (
	// KubeadmControlPlaneControllerName defines the controller used when creating clients.
	KubeadmControlPlaneControllerName = "kubeadm-controlplane-controller"
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	client.Reader

	GetMachinesForCluster(ctx context.Context, cluster *clusterv1.Cluster, filters ...collections.Func) (collections.Machines, error)
	GetMachinePoolsForCluster(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.MachinePoolList, error)
	GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey) (WorkloadCluster, error)
}

// Management holds operations on the management cluster.
type Management struct {
	Client              client.Reader
	SecretCachingClient client.Reader
	Tracker             *remote.ClusterCacheTracker
	EtcdDialTimeout     time.Duration
	EtcdCallTimeout     time.Duration
}

// RemoteClusterConnectionError represents a failure to connect to a remote cluster.
type RemoteClusterConnectionError struct {
	Name string
	Err  error
}

// Error satisfies the error interface.
func (e *RemoteClusterConnectionError) Error() string { return e.Name + ": " + e.Err.Error() }

// Unwrap satisfies the unwrap error inteface.
func (e *RemoteClusterConnectionError) Unwrap() error { return e.Err }

// Get implements client.Reader.
func (m *Management) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return m.Client.Get(ctx, key, obj, opts...)
}

// List implements client.Reader.
func (m *Management) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.Client.List(ctx, list, opts...)
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(ctx context.Context, cluster *clusterv1.Cluster, filters ...collections.Func) (collections.Machines, error) {
	return collections.GetFilteredMachinesForCluster(ctx, m.Client, cluster, filters...)
}

// GetMachinePoolsForCluster returns a list of machine pools owned by the cluster.
func (m *Management) GetMachinePoolsForCluster(ctx context.Context, cluster *clusterv1.Cluster) (*clusterv1.MachinePoolList, error) {
	selectors := []client.ListOption{
		client.InNamespace(cluster.GetNamespace()),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: cluster.GetName(),
		},
	}
	machinePoolList := &clusterv1.MachinePoolList{}
	err := m.Client.List(ctx, machinePoolList, selectors...)
	return machinePoolList, err
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context, clusterKey client.ObjectKey) (WorkloadCluster, error) {
	// TODO(chuckha): Inject this dependency.
	// TODO(chuckha): memoize this function. The workload client only exists as long as a reconciliation loop.
	restConfig, err := m.Tracker.GetRESTConfig(ctx, clusterKey)
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}
	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = 30 * time.Second

	c, err := m.Tracker.GetClient(ctx, clusterKey)
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
		clientKey, err := m.Tracker.GetEtcdClientCertificateKey(ctx, clusterKey)
		if err != nil {
			return nil, err
		}

		clientCert, err = generateClientCert(crtData, keyData, clientKey)
		if err != nil {
			return nil, err
		}
	} else {
		clientCert, err = m.getAPIServerEtcdClientCert(ctx, clusterKey)
		if err != nil {
			return nil, err
		}
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(crtData)
	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsConfig.InsecureSkipVerify = true
	return &Workload{
		restConfig:          restConfig,
		Client:              c,
		CoreDNSMigrator:     &CoreDNSMigrator{},
		etcdClientGenerator: NewEtcdClientGenerator(restConfig, tlsConfig, m.EtcdDialTimeout, m.EtcdCallTimeout),
	}, nil
}

func (m *Management) getEtcdCAKeyPair(ctx context.Context, clusterKey client.ObjectKey) ([]byte, []byte, error) {
	etcdCASecret := &corev1.Secret{}
	etcdCAObjectKey := client.ObjectKey{
		Namespace: clusterKey.Namespace,
		Name:      fmt.Sprintf("%s-etcd", clusterKey.Name),
	}

	// Try to get the certificate via the cached client.
	err := m.SecretCachingClient.Get(ctx, etcdCAObjectKey, etcdCASecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			// Return error if we got an errors which is not a NotFound error.
			return nil, nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", etcdCAObjectKey.Namespace, etcdCAObjectKey.Name)
		}

		// Try to get the certificate via the uncached client.
		if err := m.Client.Get(ctx, etcdCAObjectKey, etcdCASecret); err != nil {
			return nil, nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", etcdCAObjectKey.Namespace, etcdCAObjectKey.Name)
		}
	}

	crtData, ok := etcdCASecret.Data[secret.TLSCrtDataName]
	if !ok {
		return nil, nil, errors.Errorf("etcd tls crt does not exist for cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
	}
	keyData := etcdCASecret.Data[secret.TLSKeyDataName]
	return crtData, keyData, nil
}

func (m *Management) getAPIServerEtcdClientCert(ctx context.Context, clusterKey client.ObjectKey) (tls.Certificate, error) {
	apiServerEtcdClientCertificateSecret := &corev1.Secret{}
	apiServerEtcdClientCertificateObjectKey := client.ObjectKey{
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
