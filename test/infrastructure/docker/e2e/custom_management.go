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

package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	stdruntime "runtime"
	"strings"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/cluster-api/test/framework/management/kind"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kindv1 "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
)

// Shells out to `docker`, `kind`, `kubectl`

// CAPDCluster wraps a Cluster and has custom logic for GetWorkloadClient and setup.
// This demonstrates how to use the built-in kind management cluster with custom logic.
type CAPDCluster struct {
	*kind.Cluster
}

// NewClusterForCAPD creates a custom kind cluster with some necessary configuration to run CAPD.
func NewClusterForCAPD(ctx context.Context, name string, scheme *runtime.Scheme, images ...string) (*CAPDCluster, error) {
	cfg := &kindv1.Cluster{
		TypeMeta: kindv1.TypeMeta{
			APIVersion: "kind.x-k8s.io/v1alpha4",
			Kind:       "Cluster",
		},
	}
	kindv1.SetDefaultsCluster(cfg)
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

	// Do not use kubernetes' yaml marshaller as it doesn't respect struct tags.
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	f, err := ioutil.TempFile("", "capi-test-framework")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer os.RemoveAll(f.Name())
	if _, err := f.Write(b); err != nil {
		return nil, errors.WithStack(err)
	}
	cluster, err := kind.NewClusterWithConfig(ctx, name, f.Name(), scheme, images...)
	if err != nil {
		return nil, err
	}
	return &CAPDCluster{
		cluster,
	}, nil
}

// GetWorkloadClient uses some special logic for darwin architecture due to Docker for Mac limitations.
func (c *CAPDCluster) GetWorkloadClient(ctx context.Context, namespace, name string) (client.Client, error) {
	kubeconfigPath, err := c.GetWorkerKubeconfigPath(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	master := ""

	if stdruntime.GOOS == "darwin" {
		// TODO: This is a stop gap.
		// Bugs with same name different namespace cluster
		port, err := findLoadBalancerPort(ctx, name)
		if err != nil {
			return nil, err
		}
		masterURL := &url.URL{
			Scheme: "https",
			Host:   "127.0.0.1:" + port,
		}
		master = masterURL.String()
	}

	restConfig, err := clientcmd.BuildConfigFromFlags(master, kubeconfigPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c.ClientFromRestConfig(restConfig)
}

func findLoadBalancerPort(ctx context.Context, name string) (string, error) {
	loadBalancerName := name + "-lb"
	portFormat := `{{index (index (index .NetworkSettings.Ports "6443/tcp") 0) "HostPort"}}`
	getPathCmd := exec.NewCommand(
		exec.WithCommand("docker"),
		exec.WithArgs("inspect", loadBalancerName, "--format", portFormat),
	)
	stdout, stderr, err := getPathCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}
