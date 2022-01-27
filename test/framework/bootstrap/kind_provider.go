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

package bootstrap

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	kindv1 "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	kind "sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/exec"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

const (
	// DefaultNodeImageRepository is the default node image repository to be used for testing.
	DefaultNodeImageRepository = "kindest/node"

	// DefaultNodeImageVersion is the default Kubernetes version to be used for creating a kind cluster.
	DefaultNodeImageVersion = "v1.23.3"
)

// KindClusterOption is a NewKindClusterProvider option.
type KindClusterOption interface {
	apply(*KindClusterProvider)
}

type kindClusterOptionAdapter func(*KindClusterProvider)

func (adapter kindClusterOptionAdapter) apply(kindClusterProvider *KindClusterProvider) {
	adapter(kindClusterProvider)
}

// WithNodeImage implements a New Option that instruct the kindClusterProvider to use a specific node image / Kubernetes version.
func WithNodeImage(image string) KindClusterOption {
	return kindClusterOptionAdapter(func(k *KindClusterProvider) {
		k.nodeImage = image
	})
}

// WithDockerSockMount implements a New Option that instruct the kindClusterProvider to mount /var/run/docker.sock into
// the new kind cluster.
func WithDockerSockMount() KindClusterOption {
	return kindClusterOptionAdapter(func(k *KindClusterProvider) {
		k.withDockerSock = true
	})
}

// WithIPv6Family implements a New Option that instruct the kindClusterProvider to set the IPFamily to IPv6 in
// the new kind cluster.
func WithIPv6Family() KindClusterOption {
	return kindClusterOptionAdapter(func(k *KindClusterProvider) {
		k.ipFamily = clusterv1.IPv6IPFamily
	})
}

// LogFolder implements a New Option that instruct the kindClusterProvider to dump bootstrap logs in a folder in case of errors.
func LogFolder(path string) KindClusterOption {
	return kindClusterOptionAdapter(func(k *KindClusterProvider) {
		k.logFolder = path
	})
}

// NewKindClusterProvider returns a ClusterProvider that can create a kind cluster.
func NewKindClusterProvider(name string, options ...KindClusterOption) *KindClusterProvider {
	Expect(name).ToNot(BeEmpty(), "name is required for NewKindClusterProvider")

	clusterProvider := &KindClusterProvider{
		name: name,
	}
	for _, option := range options {
		option.apply(clusterProvider)
	}
	return clusterProvider
}

// KindClusterProvider implements a ClusterProvider that can create a kind cluster.
type KindClusterProvider struct {
	name           string
	withDockerSock bool
	kubeconfigPath string
	nodeImage      string
	ipFamily       clusterv1.ClusterIPFamily
	logFolder      string
}

// Create a Kubernetes cluster using kind.
func (k *KindClusterProvider) Create(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")

	// Sets the kubeconfig path to a temp file.
	// NB. the ClusterProvider is responsible for the cleanup of this file
	f, err := os.CreateTemp("", "e2e-kind")
	Expect(err).ToNot(HaveOccurred(), "Failed to create kubeconfig file for the kind cluster %q", k.name)
	k.kubeconfigPath = f.Name()

	// Creates the kind cluster
	k.createKindCluster()
}

// createKindCluster calls the kind library taking care of passing options for:
// - use a dedicated kubeconfig file (test should not alter the user environment)
// - if required, mount /var/run/docker.sock.
func (k *KindClusterProvider) createKindCluster() {
	kindCreateOptions := []kind.CreateOption{
		kind.CreateWithKubeconfigPath(k.kubeconfigPath),
	}

	cfg := &kindv1.Cluster{
		TypeMeta: kindv1.TypeMeta{
			APIVersion: "kind.x-k8s.io/v1alpha4",
			Kind:       "Cluster",
		},
	}

	if k.ipFamily == clusterv1.IPv6IPFamily {
		cfg.Networking.IPFamily = kindv1.IPv6Family
	}
	kindv1.SetDefaultsCluster(cfg)

	if k.withDockerSock {
		setDockerSockConfig(cfg)
	}

	kindCreateOptions = append(kindCreateOptions, kind.CreateWithV1Alpha4Config(cfg))

	nodeImage := fmt.Sprintf("%s:%s", DefaultNodeImageRepository, DefaultNodeImageVersion)
	if k.nodeImage != "" {
		nodeImage = k.nodeImage
	}
	kindCreateOptions = append(kindCreateOptions, kind.CreateWithNodeImage(nodeImage))
	kindCreateOptions = append(kindCreateOptions, kind.CreateWithRetain(true))

	provider := kind.NewProvider(kind.ProviderWithLogger(cmd.NewLogger()))
	err := provider.Create(k.name, kindCreateOptions...)
	if err != nil {
		// if requested, dump kind logs
		if k.logFolder != "" {
			if err := provider.CollectLogs(k.name, k.logFolder); err != nil {
				log.Logf("Failed to collect logs from kind: %v", err)
			}
		}

		errStr := fmt.Sprintf("Failed to create kind cluster %q: %v", k.name, err)
		// Extract the details of the RunError, if the cluster creation was triggered by a RunError.
		var runErr *exec.RunError
		if errors.As(err, &runErr) {
			errStr += "\n" + string(runErr.Output)
		}
		Expect(err).ToNot(HaveOccurred(), errStr)
	}
}

// setDockerSockConfig returns a kind config for mounting /var/run/docker.sock into the kind node.
func setDockerSockConfig(cfg *kindv1.Cluster) {
	cfg.Nodes = []kindv1.Node{
		{
			Role: kindv1.ControlPlaneRole,
			ExtraMounts: []kindv1.Mount{
				{
					HostPath:      "/var/run/docker.sock",
					ContainerPath: "/var/run/docker.sock",
				},
			},
		},
	}
}

// GetKubeconfigPath returns the path to the kubeconfig file for the cluster.
func (k *KindClusterProvider) GetKubeconfigPath() string {
	return k.kubeconfigPath
}

// Dispose the kind cluster and its kubeconfig file.
func (k *KindClusterProvider) Dispose(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Dispose")

	if err := kind.NewProvider().Delete(k.name, k.kubeconfigPath); err != nil {
		log.Logf("Deleting the kind cluster %q failed. You may need to remove this by hand.", k.name)
	}
	if err := os.Remove(k.kubeconfigPath); err != nil {
		log.Logf("Deleting the kubeconfig file %q file. You may need to remove this by hand.", k.kubeconfigPath)
	}
}
