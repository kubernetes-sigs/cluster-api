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

package kubetest

import (
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/test/framework"
)

const (
	standardImage   = "gcr.io/k8s-artifacts-prod/conformance"
	ciArtifactImage = "gcr.io/kubernetes-ci-images/conformance"
)

const (
	DefaultGinkgoNodes            = 1
	DefaultGinkoSlowSpecThreshold = 120
)

type RunInput struct {
	// ClusterProxy is a clusterctl test framework proxy for the workload cluster
	// for which to run kubetest against
	ClusterProxy framework.ClusterProxy
	// NumberOfNodes is the number of cluster nodes that exist for kubetest
	// to be aware of
	NumberOfNodes int
	// UseCIArtifacts will fetch the latest build from the main branch of kubernetes/kubernetes
	UseCIArtifacts bool `json:"useCIArtifacts,omitempty"`
	// ArtifactsDirectory is where conformance suite output will go
	ArtifactsDirectory string `json:"artifactsDirectory,omitempty"`
	// Path to the kubetest e2e config file
	ConfigFilePath string `json:"configFilePath,omitempty"`
	// GinkgoNodes is the number of Ginkgo nodes to use
	GinkgoNodes int `json:"ginkgoNodes,omitempty"`
	// GinkgoSlowSpecThreshold is time in s before spec is marked as slow
	GinkgoSlowSpecThreshold int `json:"ginkgoSlowSpecThreshold,omitempty"`
	// KubernetesVersion is the version of Kubernetes to test
	KubernetesVersion string
	// ConformanceImage is an optional field to specify an exact conformance image
	ConformanceImage string
}

// Run executes kube-test given an artifact directory, and sets settings
// required for kubetest to work with Cluster API. JUnit files are
// also gathered for inclusion in Prow.
func Run(input RunInput) error {
	if input.GinkgoNodes == 0 {
		input.GinkgoNodes = DefaultGinkgoNodes
	}
	if input.GinkgoSlowSpecThreshold == 0 {
		input.GinkgoSlowSpecThreshold = 120
	}
	if input.NumberOfNodes == 0 {
		numNodes, err := countClusterNodes(input.ClusterProxy)
		if err != nil {
			return err
		}
		input.NumberOfNodes = numNodes
	}
	if input.ArtifactsDirectory == "" {
		if dir, ok := os.LookupEnv("ARTIFACTS"); ok {
			input.ArtifactsDirectory = dir
		}
		input.ArtifactsDirectory = "_artifacts"
	}
	reportDir := path.Join(input.ArtifactsDirectory, "kubetest")
	outputDir := path.Join(reportDir, "e2e-output")
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return err
	}
	ginkgoVars := map[string]string{
		"nodes":             strconv.Itoa(input.GinkgoNodes),
		"slowSpecThreshold": strconv.Itoa(input.GinkgoSlowSpecThreshold),
	}
	e2eVars := map[string]string{
		"kubeconfig":           "/tmp/kubeconfig",
		"provider":             "skeleton",
		"report-dir":           "/output",
		"e2e-output-dir":       "/output/e2e-output",
		"dump-logs-on-failure": "false",
		"report-prefix":        "kubetest.",
		"num-nodes":            strconv.FormatInt(int64(input.NumberOfNodes), 10),
		"viper-config":         "/tmp/viper-config.yaml",
	}
	ginkgoArgs := buildArgs(ginkgoVars, "-")
	e2eArgs := buildArgs(e2eVars, "--")
	if input.ConformanceImage == "" {
		input.ConformanceImage = versionToConformanceImage(input.KubernetesVersion, input.UseCIArtifacts)
	}
	kubeConfigVolumeMount := volumeArg(input.ClusterProxy.GetKubeconfigPath(), "/tmp/kubeconfig")
	outputVolumeMount := volumeArg(reportDir, "/output")
	viperVolumeMount := volumeArg(input.ConfigFilePath, "/tmp/viper-config.yaml")
	user, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "unable to determine current user")
	}
	userArg := user.Uid + ":" + user.Gid
	e2eCmd := exec.Command("docker", "run", "--user", userArg, kubeConfigVolumeMount, outputVolumeMount, viperVolumeMount, "-t", input.ConformanceImage)
	e2eCmd.Args = append(e2eCmd.Args, "/usr/local/bin/ginkgo")
	e2eCmd.Args = append(e2eCmd.Args, ginkgoArgs...)
	e2eCmd.Args = append(e2eCmd.Args, "/usr/local/bin/e2e.test")
	e2eCmd.Args = append(e2eCmd.Args, "--")
	e2eCmd.Args = append(e2eCmd.Args, e2eArgs...)
	e2eCmd = framework.CompleteCommand(e2eCmd, "Running e2e test", false)
	if err := e2eCmd.Run(); err != nil {
		return errors.Wrap(err, "Unable to run conformance tests")
	}
	if err := framework.GatherJUnitReports(reportDir, input.ArtifactsDirectory); err != nil {
		return err
	}
	return nil
}

func countClusterNodes(proxy framework.ClusterProxy) (int, error) {
	nodeList, err := proxy.GetClientSet().CoreV1().Nodes().List(corev1.ListOptions{})
	if err != nil {
		return 0, errors.Wrap(err, "Unable to count nodes")
	}
	return len(nodeList.Items), nil
}

func isSELinuxEnforcing() bool {
	dat, err := ioutil.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return false
	}
	return string(dat) == "1"
}

func volumeArg(src, dest string) string {
	volumeArg := "-v" + src + ":" + dest
	if isSELinuxEnforcing() {
		return volumeArg + ":z"
	}
	return volumeArg
}

func versionToConformanceImage(kubernetesVersion string, usingCIArtifacts bool) string {
	k8sVersion := strings.ReplaceAll(kubernetesVersion, "+", "_")
	if usingCIArtifacts {
		return ciArtifactImage + ":" + k8sVersion
	}
	return standardImage + ":" + k8sVersion
}

// buildArgs converts a string map to the format --key=value
func buildArgs(kv map[string]string, flagMarker string) []string {
	args := make([]string, len(kv))
	i := 0
	for k, v := range kv {
		args[i] = flagMarker + k + "=" + v
		i++
	}
	return args
}
