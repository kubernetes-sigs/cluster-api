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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDocker(t *testing.T) {
	RegisterFailHandler(Fail)
	junitPath := fmt.Sprintf("junit.e2e_suite.%d.xml", ginkgoConfig.GinkgoConfig.ParallelNode)
	artifactPath, exists := os.LookupEnv("ARTIFACTS")
	if exists {
		junitPath = path.Join(artifactPath, junitPath)
	}
	junitReporter := reporters.NewJUnitReporter(junitPath)
	RunSpecsWithDefaultAndCustomReporters(t, "CAPD e2e Suite", []Reporter{junitReporter})
}

var (
	mgmt       *CAPDCluster
	ctx        = context.Background()
	config     *framework.Config
	configPath string
	logPath    string
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "e2e.conf", "path to the e2e config file")
}

var _ = BeforeSuite(func() {
	By("loading e2e config")
	configData, err := ioutil.ReadFile(configPath)
	Expect(err).ShouldNot(HaveOccurred())
	config, err = framework.LoadConfig(configData)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(config).ShouldNot(BeNil())

	By("creating the logs directory")
	artifactPath := os.Getenv("ARTIFACTS")
	logPath = path.Join(artifactPath, "logs")
	Expect(os.MkdirAll(filepath.Dir(logPath), 0755)).To(Succeed())

	By("initializing the scheme")
	scheme := runtime.NewScheme()
	Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	By("initialzing the management cluster name")
	config.ManagementClusterName = os.Getenv("CAPI_MGMT_CLUSTER_NAME")
	if config.ManagementClusterName == "" {
		config.ManagementClusterName = "docker-e2e-" + util.RandomString(6)
	}

	managementCluster := framework.InitManagementCluster(
		ctx, &framework.InitManagementClusterInput{
			Config: *config,
			Scheme: scheme,
			NewManagementClusterFn: func() (framework.ManagementCluster, error) {
				return NewClusterForCAPD(ctx, config.ManagementClusterName, scheme)
			},
		})
	Expect(managementCluster).ToNot(BeNil())
	Expect(managementCluster).To(BeAssignableToTypeOf(&CAPDCluster{}))
	mgmt = managementCluster.(*CAPDCluster)

	fmt.Printf("export KUBECONFIG=%q\n", mgmt.KubeconfigPath)
})

var _ = AfterSuite(func() {
	Expect(writeLogs(mgmt, "capi-system", "capi-controller-manager", logPath)).To(Succeed())
	Expect(writeLogs(mgmt, "capi-kubeadm-bootstrap-system", "capi-kubeadm-bootstrap-controller-manager", logPath)).To(Succeed())
	Expect(writeLogs(mgmt, "capi-kubeadm-control-plane-system", "capi-kubeadm-control-plane-controller-manager", logPath)).To(Succeed())
	Expect(writeLogs(mgmt, "capd-system", "capd-controller-manager", logPath)).To(Succeed())
	By("Deleting the management cluster")
	// If any part of teardown fails it will print what must be manually cleaned up
	mgmt.Teardown(ctx)
})

func ensureDockerArtifactsDeleted(input *framework.ControlplaneClusterInput) {
	By("Ensuring docker artifacts have been deleted")
	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	opt := &client.ListOptions{LabelSelector: lbl}

	dcl := &infrav1.DockerClusterList{}
	Expect(mgmtClient.List(ctx, dcl, opt)).To(Succeed())
	Expect(dcl.Items).To(HaveLen(0))

	dml := &infrav1.DockerMachineList{}
	Expect(mgmtClient.List(ctx, dml, opt)).To(Succeed())
	Expect(dml.Items).To(HaveLen(0))

	dmtl := &infrav1.DockerMachineTemplateList{}
	Expect(mgmtClient.List(ctx, dmtl, opt)).To(Succeed())
	Expect(dmtl.Items).To(HaveLen(0))
	By("Succeeding in deleting all docker artifacts")
}

func writeLogs(mgmt *CAPDCluster, namespace, deploymentName, logDir string) error {
	c, err := mgmt.GetClient()
	if err != nil {
		return err
	}
	clientSet, err := mgmt.GetClientSet()
	if err != nil {
		return err
	}
	deployment := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: deploymentName}, deployment); err != nil {
		return err
	}

	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	if err != nil {
		return err
	}

	pods := &corev1.PodList{}
	if err := c.List(context.TODO(), pods, client.InNamespace(namespace), client.MatchingLabels(selector)); err != nil {
		return err
	}

	for _, pod := range pods.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			logFile := path.Join(logDir, deploymentName, pod.Name, container.Name+".log")
			fmt.Fprintf(GinkgoWriter, "Creating directory: %s\n", filepath.Dir(logFile))
			if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
				return errors.Wrapf(err, "error making logDir %q", filepath.Dir(logFile))
			}

			opts := &corev1.PodLogOptions{
				Container: container.Name,
				Follow:    false,
			}

			podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(pod.Name, opts).Stream()
			if err != nil {
				return errors.Wrapf(err, "error getting pod stream for pod name %q/%q", pod.Namespace, pod.Name)
			}
			defer podLogs.Close()

			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return errors.Wrapf(err, "error opening created logFile %q", logFile)
			}
			defer f.Close()

			logs, err := ioutil.ReadAll(podLogs)
			if err != nil {
				return errors.Wrapf(err, "failed to read podLogs %q/%q", pod.Namespace, pod.Name)
			}
			if err := ioutil.WriteFile(f.Name(), logs, 0644); err != nil {
				return errors.Wrapf(err, "error writing pod logFile %q", f.Name())
			}
		}
	}
	return nil
}
