// +build e2e

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

package e2e_test

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/helpers/kind"
	"sigs.k8s.io/cluster-api/test/helpers/scheme"
	"sigs.k8s.io/cluster-api/util"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "e2e Suite")
}

const (
	capiNamespace      = "capi-system"
	capiDeploymentName = "capi-controller-manager"
	setupTimeout       = 10 * 60
)

var (
	managerImage    = flag.String("managerImage", "", "Docker image to load into the kind cluster for testing")
	capiComponents  = flag.String("capiComponents", "", "URL to CAPI components to load")
	kustomizeBinary = flag.String("kustomizeBinary", "kustomize", "path to the kustomize binary")

	kindCluster kind.Cluster
	kindClient  crclient.Client
	suiteTmpDir string
)

var _ = ginkgo.BeforeSuite(func() {
	fmt.Fprintf(ginkgo.GinkgoWriter, "Setting up kind cluster\n")

	var err error
	suiteTmpDir, err = ioutil.TempDir("", "capi-e2e-suite")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	kindCluster = kind.Cluster{
		Name: "capi-test-" + util.RandomString(6),
	}
	kindCluster.Setup()
	loadManagerImage(kindCluster)

	// Deploy the CAPA components
	deployCAPIComponents(kindCluster)

	kindClient, err = crclient.New(kindCluster.RestConfig(), crclient.Options{Scheme: scheme.SetupScheme()})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Verify capi components are deployed
	waitDeployment(capiNamespace, capiDeploymentName)
}, setupTimeout)

var _ = ginkgo.AfterSuite(func() {
	fmt.Fprintf(ginkgo.GinkgoWriter, "Tearing down kind cluster\n")
	kindCluster.Teardown()
	os.RemoveAll(suiteTmpDir)
})

func waitDeployment(namespace, name string) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "Ensuring %s/%s is deployed\n", namespace, name)
	gomega.Eventually(
		func() (int32, error) {
			deployment := &appsv1.Deployment{}
			if err := kindClient.Get(context.TODO(), apimachinerytypes.NamespacedName{Namespace: namespace, Name: name}, deployment); err != nil {
				return 0, err
			}
			return deployment.Status.ReadyReplicas, nil
		}, 5*time.Minute, 15*time.Second,
	).ShouldNot(gomega.BeZero())
}

func loadManagerImage(kindCluster kind.Cluster) {
	if managerImage != nil && *managerImage != "" {
		kindCluster.LoadImage(*managerImage)
	}
}

func applyManifests(kindCluster kind.Cluster, manifests *string) {
	gomega.Expect(manifests).ToNot(gomega.BeNil())
	fmt.Fprintf(ginkgo.GinkgoWriter, "Applying manifests for %s\n", *manifests)
	gomega.Expect(*manifests).ToNot(gomega.BeEmpty())
	kindCluster.ApplyYAML(*manifests)
}

func deployCAPIComponents(kindCluster kind.Cluster) {
	if capiComponents != nil && *capiComponents != "" {
		applyManifests(kindCluster, capiComponents)
		return
	}

	fmt.Fprintf(ginkgo.GinkgoWriter, "Generating CAPI manifests\n")

	// Build the manifests using kustomize
	capiManifests, err := exec.Command(*kustomizeBinary, "build", "../../config/default").Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Error: %s\n", string(exitError.Stderr))
		}
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// write out the manifests
	manifestFile := path.Join(suiteTmpDir, "cluster-api-components.yaml")
	gomega.Expect(ioutil.WriteFile(manifestFile, capiManifests, 0644)).To(gomega.Succeed())

	// apply generated manifests
	applyManifests(kindCluster, &manifestFile)
}
