// +build e2e

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
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/generators"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	dockerv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

func TestDocker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docker Suite")
}

var (
	mgmt *CAPDCluster
	ctx  = context.Background()
)

var _ = BeforeSuite(func() {
	managerImage := os.Getenv("MANAGER_IMAGE")
	if managerImage == "" {
		managerImage = "gcr.io/k8s-staging-capi-docker/capd-manager-amd64:dev"
	}
	capiImage := os.Getenv("CAPI_IMAGE")
	if capiImage == "" {
		capiImage = "gcr.io/k8s-staging-cluster-api/cluster-api-controller:master"
	}
	By("setting up in BeforeSuite")
	var err error

	// Set up the provider component generators based on master
	core := &generators.ClusterAPI{GitRef: "master"}
	// set up capd components based on current files
	infra := &provider{}

	// set up cert manager
	cm := &generators.CertManager{ReleaseVersion: "v0.11.1"}

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(capiv1.AddToScheme(scheme)).To(Succeed())
	Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())
	Expect(infrav1.AddToScheme(scheme)).To(Succeed())
	Expect(controlplanev1.AddToScheme(scheme)).To(Succeed())

	// Create the management cluster
	kindClusterName := os.Getenv("CAPI_MGMT_CLUSTER_NAME")
	if kindClusterName == "" {
		kindClusterName = "docker-e2e-" + util.RandomString(6)
	}
	mgmt, err = NewClusterForCAPD(ctx, kindClusterName, scheme, managerImage, capiImage)
	Expect(err).NotTo(HaveOccurred())
	Expect(mgmt).NotTo(BeNil())

	// Install all components
	// Install the cert-manager components first as some CRDs there will be part of the other providers
	framework.InstallComponents(ctx, mgmt, cm)

	// Wait for cert manager service
	// TODO: consider finding a way to make this service name dynamic.
	framework.WaitForAPIServiceAvailable(ctx, mgmt, "v1beta1.webhook.cert-manager.io")

	framework.InstallComponents(ctx, mgmt, core, infra)
	framework.WaitForPodsReadyInNamespace(ctx, mgmt, "capi-system")
	framework.WaitForPodsReadyInNamespace(ctx, mgmt, "capd-system")
	framework.WaitForPodsReadyInNamespace(ctx, mgmt, "cert-manager")
	// TODO: maybe wait for controller components to be ready
})

var _ = AfterSuite(func() {
	Expect(mgmt.Teardown(ctx)).To(Succeed())
})

func ensureDockerArtifactsDeleted(input *framework.ControlplaneClusterInput) {
	By("ensuring docker artifacts have been deleted")
	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	opt := &client.ListOptions{LabelSelector: lbl}

	dcl := &dockerv1.DockerClusterList{}
	Expect(mgmtClient.List(ctx, dcl, opt)).To(Succeed())
	Expect(dcl.Items).To(HaveLen(0))

	dml := &dockerv1.DockerMachineList{}
	Expect(mgmtClient.List(ctx, dml, opt)).To(Succeed())
	Expect(dml.Items).To(HaveLen(0))

	dmtl := &dockerv1.DockerMachineTemplateList{}
	Expect(mgmtClient.List(ctx, dmtl, opt)).To(Succeed())
	Expect(dmtl.Items).To(HaveLen(0))
}
