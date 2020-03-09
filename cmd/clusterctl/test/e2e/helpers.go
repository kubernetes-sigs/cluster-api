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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/management/kind"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateKindCluster(configFile string) (*kind.Cluster, error) {
	var kindCluster *kind.Cluster
	var err error
	framework.TryAddDefaultSchemes(clientgoscheme.Scheme)
	Expect(infrav1.AddToScheme(clientgoscheme.Scheme)).To(Succeed())

	// create kind cluster and client
	kindClusterName := "clusterctl-e2e-test-" + util.RandomString(6)
	if configFile == "" {
		kindCluster, err = kind.NewCluster(ctx, kindClusterName, clientgoscheme.Scheme, managerImage)
	} else {
		kindCluster, err = kind.NewClusterWithConfig(ctx, kindClusterName, configFile, clientgoscheme.Scheme, managerImage)
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(kindCluster).NotTo(BeNil())
	return kindCluster, err
}

func CheckAndWaitDeploymentExists(c client.Client, namespace, name string) {
	fmt.Fprintf(GinkgoWriter, "Ensuring %s/%s is deployed\n", namespace, name)
	Eventually(
		func() (int32, error) {
			deployment := &appsv1.Deployment{}
			if err := c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, deployment); err != nil {
				return 0, err
			}
			return deployment.Status.ReadyReplicas, nil
		}, 5*time.Minute, 5*time.Second,
	).ShouldNot(BeZero())
}

func CheckAndWaitCAPITestDeploymentsExist(c client.Client) {
	CheckAndWaitDeploymentExists(c, "capi-system", "capi-controller-manager")
	CheckAndWaitDeploymentExists(c, "capi-kubeadm-bootstrap-system", "capi-kubeadm-bootstrap-controller-manager")
	CheckAndWaitDeploymentExists(c, "capi-kubeadm-control-plane-system", "capi-kubeadm-control-plane-controller-manager")
	CheckAndWaitDeploymentExists(c, "capd-system", "capd-controller-manager")
}

func createTempDir() string {
	dir, err := ioutil.TempDir("", "clusterctl-e2e")
	Expect(err).NotTo(HaveOccurred())
	return dir
}

func createLocalTestClusterCtlConfig(tmpDir, path, msg string) string {
	dst := filepath.Join(tmpDir, path)
	// Create all directories in the standard layout
	err := os.MkdirAll(filepath.Dir(dst), 0755)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dst, []byte(msg), 0644)
	Expect(err).NotTo(HaveOccurred())
	return dst
}

type testMgmtClusterInfo struct {
	mgmtCluster             *kind.Cluster
	mgmtClient              client.Client
	clusterctlConfigFile    string
	coreProvider            string
	bootstrapProviders      []string
	controlPlaneProviders   []string
	infrastructureProviders []string
}

func initTestMgmtCluster(ctx context.Context, mgmtInfo testMgmtClusterInfo) {
	var err error
	c, err := clusterctlclient.New(mgmtInfo.clusterctlConfigFile)
	Expect(err).ToNot(HaveOccurred())
	initOpt := clusterctlclient.InitOptions{
		Kubeconfig:              mgmtInfo.mgmtCluster.KubeconfigPath,
		CoreProvider:            mgmtInfo.coreProvider,
		BootstrapProviders:      mgmtInfo.bootstrapProviders,
		ControlPlaneProviders:   mgmtInfo.controlPlaneProviders,
		InfrastructureProviders: mgmtInfo.infrastructureProviders,
	}
	_, err = c.Init(initOpt)
	Expect(err).ToNot(HaveOccurred())
	framework.WaitForAPIServiceAvailable(ctx, mgmtInfo.mgmtCluster, "v1beta1.webhook.cert-manager.io")
	CheckAndWaitCAPITestDeploymentsExist(mgmtInfo.mgmtClient)
}

type testWorkloadClusterInfo struct {
	workloadClusterName      string
	kubernetesVersion        string
	controlPlaneMachineCount int
	workerMachineCount       int
}

func createTestWorkloadCluster(ctx context.Context, mgmtInfo testMgmtClusterInfo, workloadInfo testWorkloadClusterInfo) {
	var err error
	c, err := clusterctlclient.New(mgmtInfo.clusterctlConfigFile)
	Expect(err).ToNot(HaveOccurred())
	options := clusterctlclient.GetClusterTemplateOptions{
		Kubeconfig:               mgmtInfo.mgmtCluster.KubeconfigPath,
		InfrastructureProvider:   mgmtInfo.infrastructureProviders[0],
		ClusterName:              workloadInfo.workloadClusterName,
		Flavor:                   "",
		KubernetesVersion:        workloadInfo.kubernetesVersion,
		ControlPlaneMachineCount: &workloadInfo.controlPlaneMachineCount,
		WorkerMachineCount:       &workloadInfo.workerMachineCount,
	}
	template, err := c.GetClusterTemplate(options)
	Expect(err).ToNot(HaveOccurred())
	yaml, err := template.Yaml()
	Expect(err).ToNot(HaveOccurred())
	// Create our workload cluster
	err = mgmtInfo.mgmtCluster.Apply(ctx, yaml)
	Expect(err).ToNot(HaveOccurred())
}
