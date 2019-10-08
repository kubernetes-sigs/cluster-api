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
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kindBinary    = flag.String("kindBinary", "kind", "path to the kind binary")
	kubectlBinary = flag.String("kubectlBinary", "kubectl", "path to the kubectl binary")
)

// Cluster represents the running state of a KIND cluster.
// An empty struct is enough to call Setup() on.
type Cluster struct {
	Name     string
	tmpDir   string
	kubepath string
}

// Setup creates a kind cluster and returns a path to the kubeconfig
func (c *Cluster) Setup() {
	var err error
	c.tmpDir, err = ioutil.TempDir("", "kind-home")
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	fmt.Fprintf(ginkgo.GinkgoWriter, "creating Kind cluster named %q\n", c.Name)
	c.run(exec.Command(*kindBinary, "create", "cluster", "--name", c.Name))
	path := c.runWithOutput(exec.Command(*kindBinary, "get", "kubeconfig-path", "--name", c.Name))
	c.kubepath = strings.TrimSpace(string(path))
	fmt.Fprintf(ginkgo.GinkgoWriter, "kubeconfig path: %q. Can use the following to access the cluster:\n", c.kubepath)
	fmt.Fprintf(ginkgo.GinkgoWriter, "export KUBECONFIG=%s\n", c.kubepath)
}

// Teardown attempts to delete the KIND cluster
func (c *Cluster) Teardown() {
	c.run(exec.Command(*kindBinary, "delete", "cluster", "--name", c.Name))
	os.RemoveAll(c.tmpDir)
}

// LoadImage loads the specified image archive into the kind cluster
func (c *Cluster) LoadImage(image string) {
	fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"loading image %q into Kind node\n",
		image)
	c.run(exec.Command(*kindBinary, "load", "docker-image", "--name", c.Name, image))
}

// ApplyYAML applies the provided manifest to the kind cluster
func (c *Cluster) ApplyYAML(manifestPath string) {
	c.run(exec.Command(
		*kubectlBinary,
		"create",
		"--kubeconfig="+c.kubepath,
		"-f", manifestPath,
	))
}

// RestConfig returns a rest configuration pointed at the provisioned cluster
func (c *Cluster) RestConfig() *restclient.Config {
	cfg, err := clientcmd.BuildConfigFromFlags("", c.kubepath)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return cfg
}

// KubeClient returns a Kubernetes client pointing at the provisioned cluster
func (c *Cluster) KubeClient() kubernetes.Interface {
	cfg := c.RestConfig()
	client, err := kubernetes.NewForConfig(cfg)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return client
}

func (c *Cluster) runWithOutput(cmd *exec.Cmd) []byte {
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	c.run(cmd)
	return stdout.Bytes()
}

func (c *Cluster) run(cmd *exec.Cmd) {
	var wg sync.WaitGroup
	errPipe, err := cmd.StderrPipe()
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())

	cmd.Env = append(
		cmd.Env,
		// KIND positions the configuration file relative to HOME.
		// To prevent clobbering an existing KIND installation, override this
		// n.b. HOME isn't always set inside BAZEL
		fmt.Sprintf("HOME=%s", c.tmpDir),
		//needed for Docker. TODO(EKF) Should be properly hermetic
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	)

	// Log output
	wg.Add(1)
	go captureOutput(&wg, errPipe, "stderr")
	if cmd.Stdout == nil {
		outPipe, err := cmd.StdoutPipe()
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
		wg.Add(1)
		go captureOutput(&wg, outPipe, "stdout")
	}

	gomega.Expect(cmd.Start()).To(gomega.Succeed())
	wg.Wait()
	gomega.Expect(cmd.Wait()).To(gomega.Succeed())
}

func captureOutput(wg *sync.WaitGroup, r io.Reader, label string) {
	defer wg.Done()
	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		fmt.Fprintf(ginkgo.GinkgoWriter, "[%s] %s", label, line)
		if err != nil {
			return
		}
	}
}
