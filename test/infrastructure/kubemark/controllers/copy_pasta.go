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

package controllers

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstrap "k8s.io/cluster-bootstrap/token/jws"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/klog"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/kubemark/util/pubkeypin"
)

const (
	DefaultClusterName     = "kubernetes"
	kubemarkName           = "hollow-node"
	DiscoveryRetryInterval = 5 * time.Second
	BootstrapUser          = "token-bootstrap-client"
	TokenUser              = "tls-bootstrap-token-user"
)

func RetrieveValidatedConfigInfo(ctx context.Context, cfg *kubeadmv1.JoinConfiguration) (*clientcmdapi.Config, error) {
	token, err := NewBootstrapTokenString(cfg.Discovery.BootstrapToken.Token)
	if err != nil {
		return nil, err
	}

	// Load the cfg.DiscoveryTokenCACertHashes into a pubkeypin.Set
	pubKeyPins := pubkeypin.NewSet()
	err = pubKeyPins.Allow(cfg.Discovery.BootstrapToken.CACertHashes...)
	if err != nil {
		return nil, err
	}

	// The function below runs for every endpoint, and all endpoints races with each other.
	// The endpoint that wins the race and completes the task first gets its kubeconfig returned below
	baseKubeConfig, err := fetchKubeConfigWithTimeout(cfg.Discovery.BootstrapToken.APIServerEndpoint, DiscoveryRetryInterval, func(endpoint string) (*clientcmdapi.Config, error) {
		insecureBootstrapConfig := buildInsecureBootstrapKubeConfig(endpoint, DefaultClusterName)
		clusterName := insecureBootstrapConfig.Contexts[insecureBootstrapConfig.CurrentContext].Cluster

		insecureClient, err := ToClientSet(insecureBootstrapConfig)
		if err != nil {
			return nil, err
		}

		klog.V(1).Infof("[discovery] Created cluster-info discovery client, requesting info from %q\n", insecureBootstrapConfig.Clusters[clusterName].Server)

		// Make an initial insecure connection to get the cluster-info ConfigMap
		insecureClusterInfo, err := insecureClient.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(ctx, bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
		if err != nil {
			klog.V(1).Infof("[discovery] Failed to request cluster info: [%s]\n", err)
			return nil, err
		}

		// Validate the MAC on the kubeconfig from the ConfigMap and load it
		insecureKubeconfigString, ok := insecureClusterInfo.Data[bootstrapapi.KubeConfigKey]
		if !ok || len(insecureKubeconfigString) == 0 {
			return nil, errors.Errorf("there is no %s key in the %s ConfigMap. This API Server isn't set up for token bootstrapping, can't connect",
				bootstrapapi.KubeConfigKey, bootstrapapi.ConfigMapClusterInfo)
		}
		detachedJWSToken, ok := insecureClusterInfo.Data[bootstrapapi.JWSSignatureKeyPrefix+token.ID]
		if !ok || len(detachedJWSToken) == 0 {
			return nil, errors.Errorf("token id %q is invalid for this cluster or it has expired. Use \"kubeadm token create\" on the control-plane node to create a new valid token", token.ID)
		}
		if !bootstrap.DetachedTokenIsValid(detachedJWSToken, insecureKubeconfigString, token.ID, token.Secret) {
			return nil, errors.New("failed to verify JWS signature of received cluster info object, can't trust this API Server")
		}
		insecureKubeconfigBytes := []byte(insecureKubeconfigString)
		insecureConfig, err := clientcmd.Load(insecureKubeconfigBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't parse the kubeconfig file in the %s configmap", bootstrapapi.ConfigMapClusterInfo)
		}

		// If no TLS root CA pinning was specified, we're done
		if pubKeyPins.Empty() {
			klog.V(1).Infof("[discovery] Cluster info signature and contents are valid and no TLS pinning was specified, will use API Server %q\n", endpoint)
			return insecureConfig, nil
		}

		// Load the cluster CA from the Config
		if len(insecureConfig.Clusters) != 1 {
			return nil, errors.Errorf("expected the kubeconfig file in the %s configmap to have a single cluster, but it had %d", bootstrapapi.ConfigMapClusterInfo, len(insecureConfig.Clusters))
		}
		var clusterCABytes []byte
		for _, cluster := range insecureConfig.Clusters {
			clusterCABytes = cluster.CertificateAuthorityData
		}
		clusterCAs, err := certutil.ParseCertsPEM(clusterCABytes)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse cluster CA from the %s configmap", bootstrapapi.ConfigMapClusterInfo)
		}

		// Validate the cluster CA public key against the pinned set
		err = pubKeyPins.CheckAny(clusterCAs)
		if err != nil {
			return nil, errors.Wrapf(err, "cluster CA found in %s configmap is invalid", bootstrapapi.ConfigMapClusterInfo)
		}

		// Now that we know the proported cluster CA, connect back a second time validating with that CA
		secureBootstrapConfig := buildSecureBootstrapKubeConfig(endpoint, clusterCABytes, clusterName)
		secureClient, err := ToClientSet(secureBootstrapConfig)
		if err != nil {
			return nil, err
		}

		klog.V(1).Infof("[discovery] Requesting info from %q again to validate TLS against the pinned public key\n", insecureBootstrapConfig.Clusters[clusterName].Server)
		secureClusterInfo, err := secureClient.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(ctx, bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
		if err != nil {
			klog.V(1).Infof("[discovery] Failed to request cluster info: [%s]\n", err)
			return nil, err
		}

		// Pull the kubeconfig from the securely-obtained ConfigMap and validate that it's the same as what we found the first time
		secureKubeconfigBytes := []byte(secureClusterInfo.Data[bootstrapapi.KubeConfigKey])
		if !bytes.Equal(secureKubeconfigBytes, insecureKubeconfigBytes) {
			return nil, errors.Errorf("the second kubeconfig from the %s configmap (using validated TLS) was different from the first", bootstrapapi.ConfigMapClusterInfo)
		}

		secureKubeconfig, err := clientcmd.Load(secureKubeconfigBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't parse the kubeconfig file in the %s configmap", bootstrapapi.ConfigMapClusterInfo)
		}

		klog.V(1).Infof("[discovery] Cluster info signature and contents are valid and TLS certificate validates against pinned roots, will use API Server %q\n", endpoint)
		return secureKubeconfig, nil
	})
	if err != nil {
		return nil, err
	}

	return baseKubeConfig, nil
}

func fetchKubeConfigWithTimeout(apiEndpoint string, discoveryTimeout time.Duration, fetchKubeConfigFunc func(string) (*clientcmdapi.Config, error)) (*clientcmdapi.Config, error) {
	stopChan := make(chan struct{})
	var resultingKubeConfig *clientcmdapi.Config
	var once sync.Once
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		wait.Until(func() {
			klog.Infof("[discovery] Trying to connect to API Server %q\n", apiEndpoint)
			cfg, err := fetchKubeConfigFunc(apiEndpoint)
			if err != nil {
				klog.Infof("[discovery] Failed to connect to API Server %q: %v\n", apiEndpoint, err)
				return
			}
			klog.Infof("[discovery] Successfully established connection with API Server %q\n", apiEndpoint)
			once.Do(func() {
				resultingKubeConfig = cfg
				close(stopChan)
			})
		}, DiscoveryRetryInterval, stopChan)
	}()

	select {
	case <-time.After(discoveryTimeout):
		once.Do(func() {
			close(stopChan)
		})
		err := errors.Errorf("abort connecting to API servers after timeout of %v", discoveryTimeout)
		klog.V(1).Infof("[discovery] %v\n", err)
		wg.Wait()
		return nil, err
	case <-stopChan:
		wg.Wait()
		return resultingKubeConfig, nil
	}
}

func NewBootstrapTokenString(token string) (*BootstrapTokenString, error) {
	substrs := bootstraputil.BootstrapTokenRegexp.FindStringSubmatch(token)
	// TODO: Add a constant for the 3 value here, and explain better why it's needed (other than because how the regexp parsin works)
	if len(substrs) != 3 {
		return nil, errors.Errorf("the bootstrap token %q was not of the form %q", token, bootstrapapi.BootstrapTokenPattern)
	}

	return &BootstrapTokenString{ID: substrs[1], Secret: substrs[2]}, nil
}

type BootstrapTokenString struct {
	ID     string
	Secret string
}

func buildInsecureBootstrapKubeConfig(endpoint, clustername string) *clientcmdapi.Config {
	controlPlaneEndpoint := fmt.Sprintf("https://%s", endpoint)
	bootstrapConfig := CreateBasic(controlPlaneEndpoint, clustername, BootstrapUser, []byte{})
	bootstrapConfig.Clusters[clustername].InsecureSkipTLSVerify = true
	return bootstrapConfig
}

func CreateBasic(serverURL, clusterName, userName string, caCert []byte) *clientcmdapi.Config {
	// Use the cluster and the username as the context name
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)

	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   serverURL,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		CurrentContext: contextName,
	}
}

func ToClientSet(config *clientcmdapi.Config) (*clientset.Clientset, error) {
	overrides := clientcmd.ConfigOverrides{Timeout: "10s"}
	clientConfig, err := clientcmd.NewDefaultClientConfig(*config, &overrides).ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API client configuration from kubeconfig")
	}

	client, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API client")
	}
	return client, nil
}

func buildSecureBootstrapKubeConfig(endpoint string, caCert []byte, clustername string) *clientcmdapi.Config {
	controlPlaneEndpoint := fmt.Sprintf("https://%s", endpoint)
	bootstrapConfig := CreateBasic(controlPlaneEndpoint, clustername, BootstrapUser, caCert)
	return bootstrapConfig
}

func CreateWithToken(serverURL, clusterName, userName string, caCert []byte, token string) *clientcmdapi.Config {
	config := CreateBasic(serverURL, clusterName, userName, caCert)
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{
		Token: token,
	}
	return config
}
