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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/generators"
	"sigs.k8s.io/cluster-api/test/framework/management/kind"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster API E2E Suite")
}

const (
	capiNamespace      = "capi-system"
	capiDeploymentName = "capi-controller-manager"
	setupTimeout       = 10 * 60
)

var (
	capiComponents = flag.String("capiComponents", "", "Path to CAPI components to load")

	kindCluster *kind.Cluster
	kindClient  client.Client
	ctx         context.Context
)

type localCAPIGenerator struct{}

func (g *localCAPIGenerator) GetName() string {
	return "Cluster API Generator: Local"
}

func (g *localCAPIGenerator) Manifests(ctx context.Context) ([]byte, error) {
	// read local out/cluster-api-components
	// this should be generated beforehand.
	if capiComponents != nil && len(*capiComponents) != 0 {
		return ioutil.ReadFile(*capiComponents)
	}
	return ioutil.ReadFile("../../out/cluster-api-components.yaml")
}

var _ = BeforeSuite(func() {
	ctx = context.Background()
	// Docker image to load into the kind cluster for testing
	managerImage := os.Getenv("MANAGER_IMAGE")
	if managerImage == "" {
		managerImage = "gcr.io/k8s-staging-cluster-api/cluster-api-controller:master"
	}
	By("setting up in BeforeSuite")
	var err error

	// Set up the local state of CAPI provider components
	capi := &localCAPIGenerator{}

	// set up cert manager generator
	cm := &generators.CertManager{ReleaseVersion: "v0.11.0"}

	Expect(corev1.AddToScheme(clientgoscheme.Scheme)).To(Succeed())
	Expect(capiv1.AddToScheme(clientgoscheme.Scheme)).To(Succeed())

	// create kind cluster
	kindClusterName := "capi-test-" + util.RandomString(6)
	kindCluster, err = kind.NewCluster(ctx, kindClusterName, clientgoscheme.Scheme, managerImage)

	Expect(err).NotTo(HaveOccurred())
	Expect(kindCluster).NotTo(BeNil())

	kindClient, err = kindCluster.GetClient()
	Expect(err).NotTo(HaveOccurred())

	// install cert manager first
	framework.InstallComponents(ctx, kindCluster, cm)
	// TODO: Can the generator handle the install and waiting for its
	// components? Instead of calling it component generator, we can call it
	// component installer.
	framework.WaitForAPIServiceAvailable(ctx, kindCluster, "v1beta1.webhook.cert-manager.io")

	// install capi components
	framework.InstallComponents(ctx, kindCluster, capi)
	framework.WaitForPodsReadyInNamespace(ctx, kindCluster, capiNamespace)

}, setupTimeout)

var _ = AfterSuite(func() {
	fmt.Fprintf(GinkgoWriter, "Tearing down kind cluster\n")
	kindCluster.Teardown(ctx)
})
