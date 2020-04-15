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

package framework

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	goruntime "runtime"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterProxy defines the behavior of a type that acts as an intermediary with an existing Kubernetes cluster.
// It should work with any Kubernetes cluster, no matter if the Cluster was created by a bootstrap.ClusterProvider,
// by Cluster API (a workload cluster or a self-hosted cluster) or else.
type ClusterProxy interface {
	// GetName returns the name of the cluster.
	GetName() string

	// GetKubeconfigPath returns the path to the kubeconfig file to be used to access the Kubernetes cluster.
	GetKubeconfigPath() string

	// GetScheme returns the scheme defining the types hosted in the Kubernetes cluster.
	// It is used when creating a controller-runtime client.
	GetScheme() *runtime.Scheme

	// GetClient returns a controller-runtime client to the Kubernetes cluster.
	GetClient() client.Client

	// GetClientSet returns a client-go client to the Kubernetes cluster.
	GetClientSet() *kubernetes.Clientset

	// Apply to apply YAML to the Kubernetes cluster, `kubectl apply`.
	Apply(context.Context, []byte) error

	// GetWorkloadCluster returns a proxy to a workload cluster defined in the Kubernetes cluster.
	GetWorkloadCluster(ctx context.Context, namespace, name string) ClusterProxy

	// Dispose proxy's internal resources (the operation does not affects the Kubernetes cluster).
	// This should be implemented as a synchronous function.
	Dispose(context.Context)
}

// clusterProxy provides a base implementation of the ClusterProxy interface.
type clusterProxy struct {
	name                    string
	kubeconfigPath          string
	scheme                  *runtime.Scheme
	shouldCleanupKubeconfig bool
}

// NewClusterProxy returns a clusterProxy given a KubeconfigPath and the scheme defining the types hosted in the cluster.
// If a kubeconfig file isn't provided, standard kubeconfig locations will be used (kubectl loading rules apply).
func NewClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) ClusterProxy {
	Expect(scheme).NotTo(BeNil(), "scheme is required for NewClusterProxy")

	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}
	return &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: false,
	}
}

// newFromAPIConfig returns a clusterProxy given a api.Config and the scheme defining the types hosted in the cluster.
func newFromAPIConfig(name string, config *api.Config, scheme *runtime.Scheme) ClusterProxy {
	// NB. the ClusterProvider is responsible for the cleanup of this file
	f, err := ioutil.TempFile("", "e2e-kubeconfig")
	Expect(err).ToNot(HaveOccurred(), "Failed to create kubeconfig file for the kind cluster %q")
	kubeconfigPath := f.Name()

	err = clientcmd.WriteToFile(*config, kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to write kubeconfig for the kind cluster to a file %q")

	return &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: true,
	}
}

// GetName returns the name of the cluster.
func (p *clusterProxy) GetName() string {
	return p.name
}

// GetKubeconfigPath returns the path to the kubeconfig file for the cluster.
func (p *clusterProxy) GetKubeconfigPath() string {
	return p.kubeconfigPath
}

// GetScheme returns the scheme defining the types hosted in the cluster.
func (p *clusterProxy) GetScheme() *runtime.Scheme {
	return p.scheme
}

// GetClient returns a controller-runtime client for the cluster.
func (p *clusterProxy) GetClient() client.Client {
	config := p.getConfig()

	c, err := client.New(config, client.Options{Scheme: p.scheme})
	Expect(err).ToNot(HaveOccurred(), "Failed to get controller-runtime client")

	return c
}

// GetClientSet returns a client-go client for the cluster.
func (p *clusterProxy) GetClientSet() *kubernetes.Clientset {
	restConfig := p.getConfig()

	cs, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to get client-go client")

	return cs
}

// Apply wraps `kubectl apply` and prints the output so we can see what gets applied to the cluster.
func (p *clusterProxy) Apply(ctx context.Context, resources []byte) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	return exec.KubectlApply(ctx, p.kubeconfigPath, resources)
}

func (p *clusterProxy) getConfig() *rest.Config {
	config, err := clientcmd.LoadFromFile(p.kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load Kubeconfig file from %q", p.kubeconfigPath)

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClientConfig from %q", p.kubeconfigPath)

	restConfig.UserAgent = "cluster-api-e2e"
	return restConfig
}

// GetWorkloadCluster returns ClusterProxy for the workload cluster.
func (p *clusterProxy) GetWorkloadCluster(ctx context.Context, namespace, name string) ClusterProxy {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetWorkloadCluster")
	Expect(namespace).NotTo(BeEmpty(), "namespace is required for GetWorkloadCluster")
	Expect(name).NotTo(BeEmpty(), "name is required for GetWorkloadCluster")

	// gets the kubeconfig from the cluster
	config := p.getKubeconfig(ctx, namespace, name)

	// if we are on mac and the cluster is a DockerCluster, it is required to fix the master address
	// by using localhost:load-balancer-host-port instead of the address used in the docker network.
	if goruntime.GOOS == "darwin" && p.isDockerCluster(ctx, namespace, name) {
		p.fixConfig(ctx, name, config)
	}

	return newFromAPIConfig(name, config, p.scheme)
}

func (p *clusterProxy) getKubeconfig(ctx context.Context, namespace string, name string) *api.Config {
	cl := p.GetClient()

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, secret)).To(Succeed(), "Failed to get %s", key)
	Expect(secret.Data).To(HaveKey("value"), "Invalid secret %s", key)

	config, err := clientcmd.Load(secret.Data["value"])
	Expect(err).ToNot(HaveOccurred(), "Failed to convert %s into a kubeconfig file", key)

	return config
}

func (p *clusterProxy) isDockerCluster(ctx context.Context, namespace string, name string) bool {
	cl := p.GetClient()

	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, cluster)).To(Succeed(), "Failed to get %s", key)

	return cluster.Spec.InfrastructureRef.Kind == "DockerCluster"
}

func (p *clusterProxy) fixConfig(ctx context.Context, name string, config *api.Config) {
	port, err := findLoadBalancerPort(ctx, name)
	Expect(err).ToNot(HaveOccurred(), "Failed to get load balancer port")

	masterURL := &url.URL{
		Scheme: "https",
		Host:   "127.0.0.1:" + port,
	}
	currentCluster := config.Contexts[config.CurrentContext].Cluster
	config.Clusters[currentCluster].Server = masterURL.String()
}

func findLoadBalancerPort(ctx context.Context, name string) (string, error) {
	loadBalancerName := name + "-lb"
	portFormat := `{{index (index (index .NetworkSettings.Ports "6443/tcp") 0) "HostPort"}}`
	getPathCmd := exec.NewCommand(
		exec.WithCommand("docker"),
		exec.WithArgs("inspect", loadBalancerName, "--format", portFormat),
	)
	stdout, _, err := getPathCmd.Run(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// Dispose clusterProxy internal resources (the operation does not affects the Kubernetes cluster).
func (p *clusterProxy) Dispose(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Dispose")

	if p.shouldCleanupKubeconfig {
		if err := os.Remove(p.kubeconfigPath); err != nil {
			fmt.Fprintf(GinkgoWriter, "Deleting the kubeconfig file %q file. You may need to remove this by hand.\n", p.kubeconfigPath)
		}
	}
}
