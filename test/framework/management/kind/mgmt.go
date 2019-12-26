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

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
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
	cmd := exec.NewCommand(
		exec.WithCommand("kind"),
		exec.WithArgs("create", "cluster", "--name", name),
	)
	return create(ctx, cmd, name, scheme, images...)
}

// NewClusterWithConfig creates a kind cluster using a kind-config file.
func NewClusterWithConfig(ctx context.Context, name, configFile string, scheme *runtime.Scheme, images ...string) (*Cluster, error) {
	cmd := exec.NewCommand(
		exec.WithCommand("kind"),
		exec.WithArgs("create", "cluster", "--name", name, "--config", configFile),
	)
	return create(ctx, cmd, name, scheme, images...)
}

func create(ctx context.Context, cmd *exec.Command, name string, scheme *runtime.Scheme, images ...string) (*Cluster, error) {
	stdout, stderr, err := cmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stdout))
		fmt.Println(string(stderr))
		return nil, err
	}
	kubeconfig, err := getKubeconfigPath(ctx, name)
	if err != nil {
		return nil, err
	}

	c := &Cluster{
		Name:                       name,
		KubeconfigPath:             kubeconfig,
		Scheme:                     scheme,
		WorkloadClusterKubeconfigs: make(map[string]string),
	}
	for _, image := range images {
		fmt.Printf("Looking for image %q locally to load to the management cluster\n", image)
		if !c.ImageExists(ctx, image) {
			fmt.Printf("Did not find image %q locally, not loading it to the management cluster\n", image)
			continue
		}
		fmt.Printf("Loading image %q on to the management cluster\n", image)
		if err := c.LoadImage(ctx, image); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// GetName returns the name of the cluster
func (c *Cluster) GetName() string {
	return c.Name
}

// LoadImage will put a local image onto the kind node
func (c *Cluster) LoadImage(ctx context.Context, image string) error {
	loadCmd := exec.NewCommand(
		exec.WithCommand("kind"),
		exec.WithArgs("load", "docker-image", image, "--name", c.Name),
	)
	stdout, stderr, err := loadCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stdout))
		fmt.Println(string(stderr))
		return err
	}
	return nil
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
	rbytes := bytes.NewReader(resources)
	applyCmd := exec.NewCommand(
		exec.WithCommand("kubectl"),
		exec.WithArgs("apply", "--kubeconfig", c.KubeconfigPath, "-f", "-"),
		exec.WithStdin(rbytes),
	)
	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	fmt.Println(string(stdout))
	return nil
}

// Wait wraps `kubectl wait`.
func (c *Cluster) Wait(ctx context.Context, args ...string) error {
	wargs := append([]string{"wait", "--kubeconfig", c.KubeconfigPath}, args...)
	wait := exec.NewCommand(
		exec.WithCommand("kubectl"),
		exec.WithArgs(wargs...),
	)
	_, stderr, err := wait.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	return nil
}

// Teardown deletes all the tmp files and cleans up the kind cluster.
func (c *Cluster) Teardown(ctx context.Context) error {
	deleteCmd := exec.NewCommand(
		exec.WithCommand("kind"),
		exec.WithArgs("delete", "cluster", "--name", c.Name),
	)
	stdout, stderr, err := deleteCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stdout))
		fmt.Println(string(stderr))
		return err
	}
	for _, f := range c.WorkloadClusterKubeconfigs {
		if err := os.RemoveAll(f); err != nil {
			fmt.Println(err)
		}
	}
	return nil
}

func getKubeconfigPath(ctx context.Context, name string) (string, error) {
	getPathCmd := exec.NewCommand(
		exec.WithCommand("kind"),
		exec.WithArgs("get", "kubeconfig-path", "--name", name),
	)
	stdout, stderr, err := getPathCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return "", err
	}
	return string(bytes.TrimSpace(stdout)), nil
}

// ClientFromRestConfig returns a controller-runtime client from a RESTConfig.
func (c *Cluster) ClientFromRestConfig(restConfig *rest.Config) (client.Client, error) {
	// Adding mapper to auto-discover schemes
	restMapper, err := apiutil.NewDynamicRESTMapper(restConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cl, err := client.New(restConfig, client.Options{
		Scheme: c.Scheme,
		Mapper: restMapper,
	})
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
// TODO Use remote package once it returns a controller runtime client
func (c *Cluster) GetWorkloadClient(ctx context.Context, namespace, name string) (client.Client, error) {
	mgmtClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	config := &v1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	if err := mgmtClient.Get(ctx, key, config); err != nil {
		return nil, err
	}

	f, err := ioutil.TempFile("", "worker-kubeconfig")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data := config.Data["value"]
	if _, err := f.Write(data); err != nil {
		return nil, errors.WithStack(err)
	}
	// TODO: remove the tmpfile and pass the secret in to clientcmd
	c.WorkloadClusterKubeconfigs[namespace+"-"+name] = f.Name()

	restConfig, err := clientcmd.BuildConfigFromFlags("", f.Name())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return c.ClientFromRestConfig(restConfig)
}
