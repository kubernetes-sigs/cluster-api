/*
Copyright 2018 The Kubernetes Authors.

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
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/util"
)

const defaultImage = "kindest/node:v1.17.2"

// Cluster represents the running state of a Kind cluster.
// An empty struct is enough to call Setup() on.
type Cluster struct {
	Name           string
	KubeconfigPath string
	Image          string
	Retain         bool
	Wait           string
	kindBinary     string
	kubectlBinary  string
}

// Setup creates a Kind cluster
func (c *Cluster) Setup() {
	c.init()
	fmt.Fprintf(GinkgoWriter, "creating Kind cluster named %q\n", c.Name)

	cmd := exec.Command( //nolint:gosec
		c.kindBinary,
		"create", "cluster",
		"--name", c.Name,
		"--image", c.Image,
		"--kubeconfig", c.KubeconfigPath,
		"--wait", c.Wait,
	)
	if c.Retain {
		cmd.Args = append(cmd.Args, "--retain")
	}
	c.run(cmd)
}

func (c *Cluster) init() {
	if c.Name == "" {
		c.Name = "kind-test-" + util.RandomString(6)
	}
	if c.KubeconfigPath == "" {
		c.KubeconfigPath = fmt.Sprintf("kubeconfig-%s", c.Name)
	}
	if c.Image == "" {
		c.Image = defaultImage
	}
	if c.Wait == "" {
		c.Wait = "0"
	}
	var err error
	c.kindBinary, err = exec.LookPath("kind")
	Expect(err).NotTo(HaveOccurred())
	c.kubectlBinary, err = exec.LookPath("kubectl")
	Expect(err).NotTo(HaveOccurred())
}

// Teardown attempts to delete the Kind cluster
func (c *Cluster) Teardown() {
	c.run(exec.Command( //nolint:gosec
		c.kindBinary,
		"delete", "cluster",
		"--name", c.Name,
	))
	os.Remove(c.KubeconfigPath)
}

// LoadImage loads the specified image archive into the Kind cluster
func (c *Cluster) LoadImage(image string) {
	fmt.Fprintf(
		GinkgoWriter,
		"loading image %q into Kind node\n",
		image)
	c.run(exec.Command( //nolint:gosec
		c.kindBinary,
		"load", "docker-image",
		"--name", c.Name,
		image,
	))
}

// ApplyYAML applies the provided manifest to the Kind cluster
func (c *Cluster) ApplyYAML(manifestPath string) {
	c.run(exec.Command( //nolint:gosec
		c.kubectlBinary,
		"apply",
		"--kubeconfig", c.KubeconfigPath,
		"-f", manifestPath,
	))
}

// RestConfig returns a REST configuration pointed at the provisioned cluster
func (c *Cluster) RestConfig() *restclient.Config {
	cfg, err := clientcmd.BuildConfigFromFlags("", c.KubeconfigPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return cfg
}

// KubeClient returns a Kubernetes client pointed at the provisioned cluster
func (c *Cluster) KubeClient() kubernetes.Interface {
	cfg := c.RestConfig()
	client, err := kubernetes.NewForConfig(cfg)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return client
}

func (c *Cluster) run(cmd *exec.Cmd) {
	var wg sync.WaitGroup
	errPipe, err := cmd.StderrPipe()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cmd.Env = append(
		cmd.Env,
		// KIND positions the configuration file relative to HOME.
		// To prevent clobbering an existing KIND installation, override this
		// n.b. HOME isn't always set inside BAZEL
		fmt.Sprintf("HOME=%s", os.Getenv("PWD")),
		//needed for Docker. TODO(EKF) Should be properly hermetic
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	)

	// Log output
	wg.Add(1)
	go captureOutput(&wg, errPipe, "stderr")
	if cmd.Stdout == nil {
		outPipe, err := cmd.StdoutPipe()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		wg.Add(1)
		go captureOutput(&wg, outPipe, "stdout")
	}

	Expect(cmd.Start()).To(Succeed())
	wg.Wait()
	Expect(cmd.Wait()).To(Succeed())
}

func captureOutput(wg *sync.WaitGroup, r io.Reader, label string) {
	defer wg.Done()
	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		fmt.Fprintf(GinkgoWriter, "[%s] %s", label, line)
		if err != nil {
			return
		}
	}
}
