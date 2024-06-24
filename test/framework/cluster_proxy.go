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
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	goruntime "runtime"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	testexec "sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
)

const (
	retryableOperationInterval = 3 * time.Second
	// retryableOperationTimeout requires a higher value especially for self-hosted upgrades.
	// Short unavailability of the Kube APIServer due to joining etcd members paired with unreachable conversion webhooks due to
	// failed leader election and thus controller restarts lead to longer taking retries.
	// The timeout occurs when listing machines in `GetControlPlaneMachinesByCluster`.
	retryableOperationTimeout = 3 * time.Minute

	initialCacheSyncTimeout = time.Minute
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

	// GetRESTConfig returns the REST config for direct use with client-go if needed.
	GetRESTConfig() *rest.Config

	// GetCache returns a controller-runtime cache to create informer from.
	GetCache(ctx context.Context) cache.Cache

	// GetLogCollector returns the machine log collector for the Kubernetes cluster.
	GetLogCollector() ClusterLogCollector

	// Apply to apply YAML to the Kubernetes cluster, `kubectl apply`.
	Apply(ctx context.Context, resources []byte, args ...string) error

	// GetWorkloadCluster returns a proxy to a workload cluster defined in the Kubernetes cluster.
	GetWorkloadCluster(ctx context.Context, namespace, name string, options ...Option) ClusterProxy

	// CollectWorkloadClusterLogs collects machines and infrastructure logs from the workload cluster.
	CollectWorkloadClusterLogs(ctx context.Context, namespace, name, outputPath string)

	// Dispose proxy's internal resources (the operation does not affects the Kubernetes cluster).
	// This should be implemented as a synchronous function.
	Dispose(context.Context)
}

// ClusterLogCollector defines an object that can collect logs from a machine.
type ClusterLogCollector interface {
	// CollectMachineLog collects log from a machine.
	// TODO: describe output folder struct
	CollectMachineLog(ctx context.Context, managementClusterClient client.Client, m *clusterv1.Machine, outputPath string) error
	CollectMachinePoolLog(ctx context.Context, managementClusterClient client.Client, m *clusterv1.MachinePool, outputPath string) error
	// CollectInfrastructureLogs collects log from the infrastructure.
	CollectInfrastructureLogs(ctx context.Context, managementClusterClient client.Client, c *clusterv1.Cluster, outputPath string) error
}

// Option is a configuration option supplied to NewClusterProxy.
type Option func(*clusterProxy)

// WithMachineLogCollector allows to define the machine log collector to be used with this Cluster.
func WithMachineLogCollector(logCollector ClusterLogCollector) Option {
	return func(c *clusterProxy) {
		c.logCollector = logCollector
	}
}

// clusterProxy provides a base implementation of the ClusterProxy interface.
type clusterProxy struct {
	name                    string
	kubeconfigPath          string
	scheme                  *runtime.Scheme
	shouldCleanupKubeconfig bool
	logCollector            ClusterLogCollector
	cache                   cache.Cache
	onceCache               sync.Once
}

// NewClusterProxy returns a clusterProxy given a KubeconfigPath and the scheme defining the types hosted in the cluster.
// If a kubeconfig file isn't provided, standard kubeconfig locations will be used (kubectl loading rules apply).
func NewClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme, options ...Option) ClusterProxy {
	Expect(scheme).NotTo(BeNil(), "scheme is required for NewClusterProxy")

	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	proxy := &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: false,
	}

	for _, o := range options {
		o(proxy)
	}

	return proxy
}

// newFromAPIConfig returns a clusterProxy given a api.Config and the scheme defining the types hosted in the cluster.
func newFromAPIConfig(name string, config *api.Config, scheme *runtime.Scheme, options ...Option) ClusterProxy {
	// NB. the ClusterProvider is responsible for the cleanup of this file
	f, err := os.CreateTemp("", "e2e-kubeconfig")
	Expect(err).ToNot(HaveOccurred(), "Failed to create kubeconfig file for the kind cluster %q")
	kubeconfigPath := f.Name()

	err = clientcmd.WriteToFile(*config, kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to write kubeconfig for the kind cluster to a file %q")

	proxy := &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: true,
	}
	for _, o := range options {
		o(proxy)
	}
	return proxy
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
	config := p.GetRESTConfig()

	var c client.Client
	var newClientErr error
	err := wait.PollUntilContextTimeout(context.TODO(), retryableOperationInterval, retryableOperationTimeout, true, func(context.Context) (bool, error) {
		c, newClientErr = client.New(config, client.Options{Scheme: p.scheme})
		if newClientErr != nil {
			return false, nil //nolint:nilerr
		}
		return true, nil
	})
	errorString := "Failed to get controller-runtime client"
	Expect(newClientErr).ToNot(HaveOccurred(), errorString)
	Expect(err).ToNot(HaveOccurred(), errorString)

	return c
}

// GetClientSet returns a client-go client for the cluster.
func (p *clusterProxy) GetClientSet() *kubernetes.Clientset {
	restConfig := p.GetRESTConfig()

	cs, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to get client-go client")

	return cs
}

func (p *clusterProxy) GetCache(ctx context.Context) cache.Cache {
	p.onceCache.Do(func() {
		var err error
		p.cache, err = cache.New(p.GetRESTConfig(), cache.Options{
			Scheme: p.scheme,
			Mapper: p.GetClient().RESTMapper(),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create controller-runtime cache")

		go func() {
			defer GinkgoRecover()
			Expect(p.cache.Start(ctx)).To(Succeed())
		}()

		cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeout(ctx, initialCacheSyncTimeout)
		defer cacheSyncCtxCancel()
		Expect(p.cache.WaitForCacheSync(cacheSyncCtx)).
			To(BeTrue(), fmt.Sprintf("failed waiting for cache for cluster proxy to sync: %v", ctx.Err()))
	})

	return p.cache
}

// Apply wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func (p *clusterProxy) Apply(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	if err := testexec.KubectlApply(ctx, p.kubeconfigPath, resources, args...); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return pkgerrors.New(fmt.Sprintf("%s: stderr: %s", err.Error(), exitErr.Stderr))
		}
		return err
	}

	return nil
}

func (p *clusterProxy) GetRESTConfig() *rest.Config {
	config, err := clientcmd.LoadFromFile(p.kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load Kubeconfig file from %q", p.kubeconfigPath)

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClientConfig from %q", p.kubeconfigPath)

	restConfig.UserAgent = "cluster-api-e2e"
	return restConfig
}

func (p *clusterProxy) GetLogCollector() ClusterLogCollector {
	return p.logCollector
}

// GetWorkloadCluster returns ClusterProxy for the workload cluster.
func (p *clusterProxy) GetWorkloadCluster(ctx context.Context, namespace, name string, options ...Option) ClusterProxy {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetWorkloadCluster")
	Expect(namespace).NotTo(BeEmpty(), "namespace is required for GetWorkloadCluster")
	Expect(name).NotTo(BeEmpty(), "name is required for GetWorkloadCluster")

	// gets the kubeconfig from the cluster
	config := p.getKubeconfig(ctx, namespace, name)

	// if we are on mac or Windows Subsystem for Linux (WSL), and the cluster is a DockerCluster, it is required to fix the control plane address
	// by using localhost:load-balancer-host-port instead of the address used in the docker network.
	if (goruntime.GOOS == "darwin" || os.Getenv("WSL_DISTRO_NAME") != "") && p.isDockerCluster(ctx, namespace, name) {
		p.fixConfig(ctx, name, config)
	}

	return newFromAPIConfig(name, config, p.scheme, options...)
}

// CollectWorkloadClusterLogs collects machines and infrastructure logs and from the workload cluster.
func (p *clusterProxy) CollectWorkloadClusterLogs(ctx context.Context, namespace, name, outputPath string) {
	if p.logCollector == nil {
		fmt.Printf("Unable to get logs for workload Cluster %s: log collector is nil.\n", klog.KRef(namespace, name))
		return
	}

	var machines *clusterv1.MachineList
	Eventually(func() error {
		var err error
		machines, err = getMachinesInCluster(ctx, p.GetClient(), namespace, name)
		return err
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get Machines for the Cluster %s", klog.KRef(namespace, name))

	for i := range machines.Items {
		m := &machines.Items[i]
		err := p.logCollector.CollectMachineLog(ctx, p.GetClient(), m, path.Join(outputPath, "machines", m.GetName()))
		if err != nil {
			// NB. we are treating failures in collecting logs as a non blocking operation (best effort)
			fmt.Printf("Failed to get logs for Machine %s, Cluster %s: %v\n", m.GetName(), klog.KRef(namespace, name), err)
		}
	}

	var machinePools *clusterv1.MachinePoolList
	Eventually(func() error {
		var err error
		machinePools, err = getMachinePoolsInCluster(ctx, p.GetClient(), namespace, name)
		return err
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get MachinePools for Cluster %s", klog.KRef(namespace, name))

	for i := range machinePools.Items {
		mp := &machinePools.Items[i]
		err := p.logCollector.CollectMachinePoolLog(ctx, p.GetClient(), mp, path.Join(outputPath, "machine-pools", mp.GetName()))
		if err != nil {
			// NB. we are treating failures in collecting logs as a non-blocking operation (best effort)
			fmt.Printf("Failed to get logs for MachinePool %s, Cluster %s: %v\n", mp.GetName(), klog.KRef(namespace, name), err)
		}
	}

	cluster := &clusterv1.Cluster{}
	Eventually(func() error {
		key := client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
		return client.IgnoreNotFound(p.GetClient().Get(ctx, key, cluster))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get Cluster %s", klog.KRef(namespace, name))

	if cluster != nil {
		err := p.logCollector.CollectInfrastructureLogs(ctx, p.GetClient(), cluster, path.Join(outputPath, "infrastructure"))
		if err != nil {
			// NB. we are treating failures in collecting logs as a non blocking operation (best effort)
			fmt.Printf("Failed to get infrastructure logs for Cluster %s: %v\n", klog.KRef(namespace, name), err)
		}
	}
}

func getMachinesInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachineList, error) {
	if name == "" {
		return nil, errors.New("cluster name should not be empty")
	}

	machineList := &clusterv1.MachineList{}
	labels := map[string]string{clusterv1.ClusterNameLabel: name}
	if err := c.List(ctx, machineList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machineList, nil
}

func getMachinePoolsInCluster(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.MachinePoolList, error) {
	if name == "" {
		return nil, errors.New("cluster name should not be empty")
	}

	machinePoolList := &clusterv1.MachinePoolList{}
	labels := map[string]string{clusterv1.ClusterNameLabel: name}
	if err := c.List(ctx, machinePoolList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return machinePoolList, nil
}

func (p *clusterProxy) getKubeconfig(ctx context.Context, namespace string, name string) *api.Config {
	cl := p.GetClient()

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	Eventually(func() error {
		return cl.Get(ctx, key, secret)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get %s", key)
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
	Eventually(func() error {
		return cl.Get(ctx, key, cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get %s", key)

	return cluster.Spec.InfrastructureRef.Kind == "DockerCluster"
}

func (p *clusterProxy) fixConfig(ctx context.Context, name string, config *api.Config) {
	containerRuntime, err := container.NewDockerClient()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Docker runtime client")
	ctx = container.RuntimeInto(ctx, containerRuntime)

	lbContainerName := name + "-lb"

	// Check if the container exists locally.
	filters := container.FilterBuilder{}
	filters.AddKeyValue("name", lbContainerName)
	containers, err := containerRuntime.ListContainers(ctx, filters)
	Expect(err).ToNot(HaveOccurred())
	if len(containers) == 0 {
		// Return without changing the config if the container does not exist locally.
		// Note: This is necessary when running the tests with Tilt and a remote Docker
		// engine as the lb container running on the remote Docker engine is accessible
		// under its normal address but not via 127.0.0.1.
		return
	}

	port, err := containerRuntime.GetHostPort(ctx, lbContainerName, "6443/tcp")
	Expect(err).ToNot(HaveOccurred(), "Failed to get load balancer port")

	controlPlaneURL := &url.URL{
		Scheme: "https",
		Host:   "127.0.0.1:" + port,
	}
	currentCluster := config.Contexts[config.CurrentContext].Cluster
	config.Clusters[currentCluster].Server = controlPlaneURL.String()
}

// Dispose clusterProxy internal resources (the operation does not affects the Kubernetes cluster).
func (p *clusterProxy) Dispose(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Dispose")

	if p.shouldCleanupKubeconfig {
		if err := os.Remove(p.kubeconfigPath); err != nil {
			log.Logf("Deleting the kubeconfig file %q file. You may need to remove this by hand.", p.kubeconfigPath)
		}
	}
}
