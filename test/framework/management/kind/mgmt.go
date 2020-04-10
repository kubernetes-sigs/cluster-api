/*
Copyright 2019 The Kubernetes Authors.

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

package kind

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/fs"

	"sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/cluster-api/test/framework/options"
)

// Shells out to `kind`, `kubectl`

// Cluster represents a Kubernetes cluster used as a management cluster backed by kind.
type Cluster struct {
	Name                       string
	KubeconfigPath             string
	Client                     client.Client
	Scheme                     *runtime.Scheme
	WorkloadClusterKubeconfigs map[string]string
	// TODO: Expose the RESTConfig and a way to create a RESTConfig for the workload clusters for static-client uses
	//       (pod logs, exec and any other subresources)
}

// NewCluster sets up a new kind cluster to be used as the management cluster.
func NewCluster(ctx context.Context, name string, scheme *runtime.Scheme, images ...string) (*Cluster, error) {
	return create(ctx, name, "", scheme, images...)
}

// NewClusterWithConfig creates a kind cluster using a kind-config file.
func NewClusterWithConfig(ctx context.Context, name, configFile string, scheme *runtime.Scheme, images ...string) (*Cluster, error) {
	return create(ctx, name, configFile, scheme, images...)
}

func create(ctx context.Context, name, configFile string, scheme *runtime.Scheme, images ...string) (*Cluster, error) {
	f, err := ioutil.TempFile("", "mgmt-kubeconfig")
	// if there is an error there will not be a file to clean up
	if err != nil {
		return nil, err
	}
	// After this point we have things to clean up, so always return a *Cluster

	// Make the cluster up front and always return it so Teardown can still run
	c := &Cluster{
		Name:                       name,
		Scheme:                     scheme,
		KubeconfigPath:             f.Name(),
		WorkloadClusterKubeconfigs: make(map[string]string),
	}

	provider := cluster.NewProvider(cluster.ProviderWithLogger(cmd.NewLogger()))
	kindConfig := cluster.CreateWithConfigFile(configFile)
	kubeConfig := cluster.CreateWithKubeconfigPath(f.Name())

	if err := provider.Create(name, kindConfig, kubeConfig); err != nil {
		return c, err
	}

	for _, image := range images {
		fmt.Printf("Looking for image %q locally to load to the management cluster\n", image)
		if !c.ImageExists(ctx, image) {
			fmt.Printf("Did not find image %q locally, not loading it to the management cluster\n", image)
			continue
		}
		fmt.Printf("Loading image %q on to the management cluster\n", image)
		if err := c.LoadImage(ctx, image); err != nil {
			return c, err
		}
	}
	return c, nil
}

// GetName returns the name of the cluster
func (c *Cluster) GetName() string {
	return c.Name
}

// GetKubeconfigPath returns the path to the kubeconfig file for the cluster.
func (c Cluster) GetKubeconfigPath() string {
	return c.KubeconfigPath
}

// GetScheme returns the scheme defining the types hosted in the cluster.
func (c Cluster) GetScheme() *runtime.Scheme {
	return c.Scheme
}

// LoadImage will put a local image onto the kind node
func (c *Cluster) LoadImage(ctx context.Context, image string) error {
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(cmd.NewLogger()),
	)

	// Save the image into a tar
	dir, err := fs.TempDir("", "image-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)
	imageTarPath := filepath.Join(dir, "image.tar")

	err = save(ctx, image, imageTarPath)
	if err != nil {
		return err
	}

	nodeList, err := provider.ListInternalNodes(c.Name)
	if err != nil {
		return err
	}

	// Load the image on the selected nodes
	for _, node := range nodeList {
		if err := loadImage(imageTarPath, node); err != nil {
			return err
		}
	}

	return nil
}

// copied from kind https://github.com/kubernetes-sigs/kind/blob/v0.7.0/pkg/cmd/kind/load/docker-image/docker-image.go#L168
// save saves image to dest, as in `docker save`
func save(ctx context.Context, image, dest string) error {
	_, _, err := exec.NewCommand(
		exec.WithCommand("docker"),
		exec.WithArgs("save", "-o", dest, image)).Run(ctx)
	return err
}

// copied from kind https://github.com/kubernetes-sigs/kind/blob/v0.7.0/pkg/cmd/kind/load/docker-image/docker-image.go#L158
// loads an image tarball onto a node
func loadImage(imageTarName string, node nodes.Node) error {
	f, err := os.Open(imageTarName)
	if err != nil {
		return errors.Wrap(err, "failed to open image")
	}
	defer f.Close()
	return nodeutils.LoadImageArchive(node, f)
}

func (c *Cluster) ImageExists(ctx context.Context, image string) bool {
	existsCmd := exec.NewCommand(
		exec.WithCommand("docker"),
		exec.WithArgs("images", "-q", image),
	)
	stdout, stderr, err := existsCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stdout))
		fmt.Println(string(stderr))
		fmt.Println(err.Error())
		return false
	}
	// Docker returns a 0 exit code regardless if the image is listed or not.
	// It will return the image ID if the image exists and nothing else otherwise.
	return len(bytes.TrimSpace(stdout)) > 0
}

// TODO: Considier a Kubectl function and then wrap it at the next level up.

// Apply wraps `kubectl apply` and prints the output so we can see what gets applied to the cluster.
func (c *Cluster) Apply(ctx context.Context, resources []byte) error {
	return exec.KubectlApply(ctx, c.KubeconfigPath, resources)
}

// Wait wraps `kubectl wait`.
func (c *Cluster) Wait(ctx context.Context, args ...string) error {
	return exec.KubectlWait(ctx, c.KubeconfigPath, args...)
}

// Teardown deletes all the tmp files and cleans up the kind cluster.
// This does not return an error so that it can clean as much up as possible regardless of error.
func (c *Cluster) Teardown(_ context.Context) {
	if options.SkipResourceCleanup {
		return
	}
	if c == nil {
		return
	}
	if err := cluster.NewProvider(cluster.ProviderWithLogger(cmd.NewLogger())).Delete(c.Name, c.KubeconfigPath); err != nil {
		fmt.Printf("Deleting the kind cluster %q failed. You may need to remove this by hand.\n", c.Name)
	}
	for _, f := range c.WorkloadClusterKubeconfigs {
		if err := os.RemoveAll(f); err != nil {
			fmt.Printf("Unable to delete a workload cluster config %q. You may need to remove this by hand.\n", f)
			fmt.Println(err)
		}
	}
	if err := os.Remove(c.KubeconfigPath); err != nil {
		fmt.Printf("Unable to remove %q. You may need to remove this by hand.\n", c.KubeconfigPath)
		fmt.Println(err)
	}
}

// ClientFromRestConfig returns a controller-runtime client from a RESTConfig.
func (c *Cluster) ClientFromRestConfig(restConfig *rest.Config) (client.Client, error) {
	cl, err := client.New(restConfig, client.Options{Scheme: c.Scheme})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c.Client = cl
	return c.Client, nil
}

// GetClientSet returns a clientset to the management cluster to be used for object interface expansions such as pod logs.
func (c *Cluster) GetClientSet() (*kubernetes.Clientset, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", c.KubeconfigPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return kubernetes.NewForConfig(restConfig)
}

// GetClient returns a controller-runtime client for the management cluster.
func (c *Cluster) GetClient() (client.Client, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", c.KubeconfigPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c.ClientFromRestConfig(restConfig)
}

// GetWorkloadClient returns a controller-runtime client for the workload cluster.
func (c *Cluster) GetWorkloadClient(ctx context.Context, namespace, name string) (client.Client, error) {
	kubeconfigPath, err := c.GetWorkerKubeconfigPath(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return c.ClientFromRestConfig(restConfig)
}

// GetWorkerKubeconfigPath returns the path to the kubeconfig file for the specified workload cluster.
func (c *Cluster) GetWorkerKubeconfigPath(ctx context.Context, namespace, name string) (string, error) {
	mgmtClient, err := c.GetClient()
	if err != nil {
		return "", err
	}
	config := &v1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	if err := mgmtClient.Get(ctx, key, config); err != nil {
		return "", err
	}

	f, err := ioutil.TempFile("", "worker-kubeconfig")
	if err != nil {
		return "", errors.WithStack(err)
	}
	data := config.Data["value"]
	if _, err := f.Write(data); err != nil {
		return "", errors.WithStack(err)
	}
	// TODO: remove the tmpfile and pass the secret in to clientcmd
	c.WorkloadClusterKubeconfigs[namespace+"-"+name] = f.Name()

	return f.Name(), nil
}
