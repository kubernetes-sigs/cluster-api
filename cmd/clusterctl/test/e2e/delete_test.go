// +build e2e

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

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
)

var _ = Describe("clusterctl delete", func() {
	var (
		mgmtInfo      testMgmtClusterInfo
		deleteOptions clusterctlclient.DeleteOptions
	)

	BeforeEach(func() {
		var err error
		// Mgmt cluster info object
		mgmtInfo = testMgmtClusterInfo{
			clusterctlConfigFile:    clusterctlConfigFile,
			coreProvider:            "cluster-api:v0.3.0",
			bootstrapProviders:      []string{"kubeadm-bootstrap:v0.3.0"},
			controlPlaneProviders:   []string{"kubeadm-control-plane:v0.3.0"},
			infrastructureProviders: []string{"docker:v0.3.0"},
		}
		// Create the mgmt cluster and client
		mgmtInfo.mgmtCluster, err = CreateKindCluster(kindConfigFile)
		Expect(err).ToNot(HaveOccurred())
		mgmtInfo.mgmtClient, err = mgmtInfo.mgmtCluster.GetClient()
		Expect(err).NotTo(HaveOccurred())

		initTestMgmtCluster(ctx, mgmtInfo)

	}, setupTimeout)

	JustBeforeEach(func() {
		c, err := clusterctlclient.New(mgmtInfo.clusterctlConfigFile)
		Expect(err).ToNot(HaveOccurred())
		err = c.Delete(deleteOptions)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		fmt.Fprintf(GinkgoWriter, "Tearing down kind clusters\n")
		mgmtInfo.mgmtCluster.Teardown(ctx)
	})

	Context("deletes the infra provider", func() {
		BeforeEach(func() {
			deleteOptions = clusterctlclient.DeleteOptions{
				Kubeconfig: mgmtInfo.mgmtCluster.KubeconfigPath,
				Providers:  []string{"docker"},
			}
		})
		It("should delete of all infra provider components except the hosting namespace and the CRDs.", func() {
			Eventually(
				func() bool {
					if !apierrors.IsNotFound(mgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "capd-system", Name: "capd-controller-manager"}, &appsv1.Deployment{})) {
						return false
					}
					// TODO: check that namespace and CRD are still present.
					return true
				}, 3*time.Minute, 5*time.Second,
			).Should(BeTrue())
		})
	})
	Context("deletes everything", func() {
		BeforeEach(func() {
			deleteOptions = clusterctlclient.DeleteOptions{
				Kubeconfig:           mgmtInfo.mgmtCluster.KubeconfigPath,
				ForceDeleteNamespace: true,
				ForceDeleteCRD:       true,
				Providers:            []string{},
			}
		})
		It("should reset the management cluster to its original state", func() {
			Eventually(
				func() bool {
					// TODO: check all components are deleted.
					if !apierrors.IsNotFound(mgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "capd-system", Name: "capd-controller-manager"}, &appsv1.Deployment{})) {
						return false
					}
					// TODO: check namespace of all components are deleted.
					if !apierrors.IsNotFound(mgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Name: "capd-system"}, &corev1.Namespace{})) {
						return false
					}
					// TODO: check that all CRDs are deleted.
					err := mgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "foo-cluster"}, &infrav1.DockerCluster{})
					if _, ok := err.(*meta.NoResourceMatchError); !ok {
						return false
					}
					return true
				}, 3*time.Minute, 5*time.Second,
			).Should(BeTrue())
		})
	})
})
