/*
Copyright 2023 The Kubernetes Authors.

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

package server

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"sigs.k8s.io/cluster-api/util/certs"
)

// WorkloadClusterListener represents a listener for a workload cluster.
type WorkloadClusterListener struct {
	host string
	port int

	resourceGroup string

	scheme *runtime.Scheme

	apiServers                  sets.Set[string]
	apiServerCaCertificate      *x509.Certificate
	apiServerCaKey              *rsa.PrivateKey
	apiServerServingCertificate *tls.Certificate

	adminCertificate *x509.Certificate
	adminKey         *rsa.PrivateKey

	etcdMembers             sets.Set[string]
	etcdServingCertificates map[string]*tls.Certificate

	listener net.Listener
}

// Host returns the host of a WorkloadClusterListener.
func (s *WorkloadClusterListener) Host() string {
	return s.host
}

// Port returns the port of a WorkloadClusterListener.
func (s *WorkloadClusterListener) Port() int {
	return s.port
}

// ResourceGroup returns the resource group that hosts in memory resources for a WorkloadClusterListener.
func (s *WorkloadClusterListener) ResourceGroup() string {
	return s.resourceGroup
}

// Address returns the address of a WorkloadClusterListener.
func (s *WorkloadClusterListener) Address() string {
	return fmt.Sprintf("https://%s", s.HostPort())
}

// HostPort returns the host port of a WorkloadClusterListener.
func (s *WorkloadClusterListener) HostPort() string {
	return net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
}

// RESTConfig returns the rest config for a WorkloadClusterListener.
func (s *WorkloadClusterListener) RESTConfig() (*rest.Config, error) {
	kubeConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"in-memory": {
				Server:                   s.Address(),
				CertificateAuthorityData: certs.EncodeCertPEM(s.apiServerCaCertificate), // TODO: convert to PEM (store in double format
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"in-memory": {
				Username:              "in-memory",
				ClientCertificateData: certs.EncodeCertPEM(s.adminCertificate), // TODO: convert to PEM
				ClientKeyData:         certs.EncodePrivateKeyPEM(s.adminKey),   // TODO: convert to PEM
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"in-memory": {
				Cluster:  "in-memory",
				AuthInfo: "in-memory",
			},
		},
		CurrentContext: "in-memory",
	}

	b, err := clientcmd.Write(kubeConfig)
	if err != nil {
		return nil, err
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(b)
	if err != nil {
		return nil, err
	}

	return restConfig, nil
}

// GetClient returns a client for a WorkloadClusterListener.
func (s *WorkloadClusterListener) GetClient() (client.WithWatch, error) {
	restConfig, err := s.RESTConfig()
	if err != nil {
		return nil, err
	}

	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, err
	}

	mapper, err := apiutil.NewDynamicRESTMapper(restConfig, httpClient)
	if err != nil {
		return nil, err
	}

	c, err := client.NewWithWatch(restConfig, client.Options{Scheme: s.scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	return c, nil
}
