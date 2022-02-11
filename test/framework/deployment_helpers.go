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

package framework

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

const (
	nodeRoleOldControlPlane = "node-role.kubernetes.io/master" // Deprecated: https://github.com/kubernetes/kubeadm/issues/2200
	nodeRoleControlPlane    = "node-role.kubernetes.io/control-plane"
)

// WaitForDeploymentsAvailableInput is the input for WaitForDeploymentsAvailable.
type WaitForDeploymentsAvailableInput struct {
	Getter     Getter
	Deployment *appsv1.Deployment
}

// WaitForDeploymentsAvailable waits until the Deployment has status.Available = True, that signals that
// all the desired replicas are in place.
// This can be used to check if Cluster API controllers installed in the management cluster are working.
func WaitForDeploymentsAvailable(ctx context.Context, input WaitForDeploymentsAvailableInput, intervals ...interface{}) {
	By(fmt.Sprintf("Waiting for deployment %s/%s to be available", input.Deployment.GetNamespace(), input.Deployment.GetName()))
	deployment := &appsv1.Deployment{}
	Eventually(func() bool {
		key := client.ObjectKey{
			Namespace: input.Deployment.GetNamespace(),
			Name:      input.Deployment.GetName(),
		}
		if err := input.Getter.Get(ctx, key, deployment); err != nil {
			return false
		}
		for _, c := range deployment.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, intervals...).Should(BeTrue(), func() string { return DescribeFailedDeployment(input, deployment) })
}

// DescribeFailedDeployment returns detailed output to help debug a deployment failure in e2e.
func DescribeFailedDeployment(input WaitForDeploymentsAvailableInput, deployment *appsv1.Deployment) string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("Deployment %s/%s failed to get status.Available = True condition",
		input.Deployment.GetNamespace(), input.Deployment.GetName()))
	if deployment == nil {
		b.WriteString("\nDeployment: nil\n")
	} else {
		b.WriteString(fmt.Sprintf("\nDeployment:\n%s\n", PrettyPrint(deployment)))
	}
	return b.String()
}

// WatchDeploymentLogsInput is the input for WatchDeploymentLogs.
type WatchDeploymentLogsInput struct {
	GetLister  GetLister
	ClientSet  *kubernetes.Clientset
	Deployment *appsv1.Deployment
	LogPath    string
}

// WatchDeploymentLogs streams logs for all containers for all pods belonging to a deployment. Each container's logs are streamed
// in a separate goroutine so they can all be streamed concurrently. This only causes a test failure if there are errors
// retrieving the deployment, its pods, or setting up a log file. If there is an error with the log streaming itself,
// that does not cause the test to fail.
func WatchDeploymentLogs(ctx context.Context, input WatchDeploymentLogsInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WatchControllerLogs")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for WatchControllerLogs")
	Expect(input.Deployment).NotTo(BeNil(), "input.Deployment is required for WatchControllerLogs")

	deployment := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(input.Deployment)
	Expect(input.GetLister.Get(ctx, key, deployment)).To(Succeed(), "Failed to get deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	Expect(err).NotTo(HaveOccurred(), "Failed to Pods selector for deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	pods := &corev1.PodList{}
	Expect(input.GetLister.List(ctx, pods, client.InNamespace(input.Deployment.Namespace), client.MatchingLabels(selector))).To(Succeed(), "Failed to list Pods for deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	for _, pod := range pods.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			log.Logf("Creating log watcher for controller %s/%s, pod %s, container %s", input.Deployment.Namespace, input.Deployment.Name, pod.Name, container.Name)

			// Watch each container's logs in a goroutine so we can stream them all concurrently.
			go func(pod corev1.Pod, container corev1.Container) {
				defer GinkgoRecover()

				logFile := filepath.Clean(path.Join(input.LogPath, input.Deployment.Name, pod.Name, container.Name+".log"))
				Expect(os.MkdirAll(filepath.Dir(logFile), 0750)).To(Succeed())

				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
				Expect(err).NotTo(HaveOccurred())
				defer f.Close()

				opts := &corev1.PodLogOptions{
					Container: container.Name,
					Follow:    true,
				}

				podLogs, err := input.ClientSet.CoreV1().Pods(input.Deployment.Namespace).GetLogs(pod.Name, opts).Stream(ctx)
				if err != nil {
					// Failing to stream logs should not cause the test to fail
					log.Logf("Error starting logs stream for pod %s/%s, container %s: %v", input.Deployment.Namespace, pod.Name, container.Name, err)
					return
				}
				defer podLogs.Close()

				out := bufio.NewWriter(f)
				defer out.Flush()
				_, err = out.ReadFrom(podLogs)
				if err != nil && err != io.ErrUnexpectedEOF {
					// Failing to stream logs should not cause the test to fail
					log.Logf("Got error while streaming logs for pod %s/%s, container %s: %v", input.Deployment.Namespace, pod.Name, container.Name, err)
				}
			}(pod, container)
		}
	}
}

type WatchPodMetricsInput struct {
	GetLister   GetLister
	ClientSet   *kubernetes.Clientset
	Deployment  *appsv1.Deployment
	MetricsPath string
}

// WatchPodMetrics captures metrics from all pods every 5s. It expects to find port 8080 open on the controller.
func WatchPodMetrics(ctx context.Context, input WatchPodMetricsInput) {
	// Dump machine metrics every 5 seconds
	ticker := time.NewTicker(time.Second * 5)
	Expect(ctx).NotTo(BeNil(), "ctx is required for dumpContainerMetrics")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for dumpContainerMetrics")
	Expect(input.Deployment).NotTo(BeNil(), "input.Deployment is required for dumpContainerMetrics")

	deployment := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(input.Deployment)
	Expect(input.GetLister.Get(ctx, key, deployment)).To(Succeed(), "Failed to get deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	Expect(err).NotTo(HaveOccurred(), "Failed to Pods selector for deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	pods := &corev1.PodList{}
	Expect(input.GetLister.List(ctx, pods, client.InNamespace(input.Deployment.Namespace), client.MatchingLabels(selector))).To(Succeed(), "Failed to list Pods for deployment %s/%s", input.Deployment.Namespace, input.Deployment.Name)

	go func() {
		defer GinkgoRecover()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dumpPodMetrics(ctx, input.ClientSet, input.MetricsPath, deployment.Name, pods)
			}
		}
	}()
}

// dumpPodMetrics captures metrics from all pods. It expects to find port 8080 open on the controller.
func dumpPodMetrics(ctx context.Context, client *kubernetes.Clientset, metricsPath string, deploymentName string, pods *corev1.PodList) {
	for _, pod := range pods.Items {
		metricsDir := path.Join(metricsPath, deploymentName, pod.Name)
		metricsFile := path.Join(metricsDir, "metrics.txt")
		Expect(os.MkdirAll(metricsDir, 0750)).To(Succeed())

		res := client.CoreV1().RESTClient().Get().
			Namespace(pod.Namespace).
			Resource("pods").
			Name(fmt.Sprintf("%s:8080", pod.Name)).
			SubResource("proxy").
			Suffix("metrics").
			Do(ctx)
		data, err := res.Raw()

		if err != nil {
			// Failing to dump metrics should not cause the test to fail
			data = []byte(fmt.Sprintf("Error retrieving metrics for pod %s/%s: %v\n%s", pod.Namespace, pod.Name, err, string(data)))
			metricsFile = path.Join(metricsDir, "metrics-error.txt")
		}

		if err := os.WriteFile(metricsFile, data, 0600); err != nil {
			// Failing to dump metrics should not cause the test to fail
			log.Logf("Error writing metrics for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		}
	}
}

// WaitForDNSUpgradeInput is the input for WaitForDNSUpgrade.
type WaitForDNSUpgradeInput struct {
	Getter     Getter
	DNSVersion string
}

// WaitForDNSUpgrade waits until CoreDNS version matches with the CoreDNS upgrade version. This is called during KCP upgrade.
func WaitForDNSUpgrade(ctx context.Context, input WaitForDNSUpgradeInput, intervals ...interface{}) {
	By("Ensuring CoreDNS has the correct image")

	Eventually(func() (bool, error) {
		d := &appsv1.Deployment{}

		if err := input.Getter.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, d); err != nil {
			return false, err
		}

		// NOTE: coredns image name has changed over time (k8s.gcr.io/coredns,
		// k8s.gcr.io/coredns/coredns), so we are checking only if the version actually changed.
		if strings.HasSuffix(d.Spec.Template.Spec.Containers[0].Image, fmt.Sprintf(":%s", input.DNSVersion)) {
			return true, nil
		}
		return false, nil
	}, intervals...).Should(BeTrue())
}

type DeployUnevictablePodInput struct {
	WorkloadClusterProxy ClusterProxy
	ControlPlane         *controlplanev1.KubeadmControlPlane
	DeploymentName       string
	Namespace            string

	WaitForDeploymentAvailableInterval []interface{}
}

func DeployUnevictablePod(ctx context.Context, input DeployUnevictablePodInput) {
	Expect(input.DeploymentName).ToNot(BeNil(), "Need a deployment name in DeployUnevictablePod")
	Expect(input.Namespace).ToNot(BeNil(), "Need a namespace in DeployUnevictablePod")
	Expect(input.WorkloadClusterProxy).ToNot(BeNil(), "Need a workloadClusterProxy in DeployUnevictablePod")
	workloadClient := input.WorkloadClusterProxy.GetClientSet()

	log.Logf("Check if namespace %s exists", input.Namespace)
	if _, err := workloadClient.CoreV1().Namespaces().Get(ctx, input.Namespace, metav1.GetOptions{}); err != nil {
		_, errCreateNamespace := workloadClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: input.Namespace,
			},
		}, metav1.CreateOptions{})
		Expect(errCreateNamespace).To(BeNil())
	}

	workloadDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.DeploymentName,
			Namespace: input.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(4),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nonstop",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "nonstop",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "web",
							Image: "nginx:1.12",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
	if input.ControlPlane != nil {
		serverVersion, err := workloadClient.ServerVersion()
		Expect(err).ToNot(HaveOccurred())

		// Use the control-plane label for Kubernetes version >= v1.20.0.
		if utilversion.MustParseGeneric(serverVersion.String()).AtLeast(utilversion.MustParseGeneric("v1.20.0")) {
			workloadDeployment.Spec.Template.Spec.NodeSelector = map[string]string{nodeRoleControlPlane: ""}
		} else {
			workloadDeployment.Spec.Template.Spec.NodeSelector = map[string]string{nodeRoleOldControlPlane: ""}
		}

		workloadDeployment.Spec.Template.Spec.Tolerations = []corev1.Toleration{
			{
				Key:    nodeRoleOldControlPlane,
				Effect: "NoSchedule",
			},
			{
				Key:    nodeRoleControlPlane,
				Effect: "NoSchedule",
			},
		}
	}
	AddDeploymentToWorkloadCluster(ctx, AddDeploymentToWorkloadClusterInput{
		Namespace:  input.Namespace,
		ClientSet:  workloadClient,
		Deployment: workloadDeployment,
	})

	budget := &v1beta1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodDisruptionBudget",
			APIVersion: "policy/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.DeploymentName,
			Namespace: input.Namespace,
		},
		Spec: v1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nonstop",
				},
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 1,
				StrVal: "1",
			},
		},
	}
	AddPodDisruptionBudget(ctx, AddPodDisruptionBudgetInput{
		Namespace: input.Namespace,
		ClientSet: workloadClient,
		Budget:    budget,
	})

	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.WorkloadClusterProxy.GetClient(),
		Deployment: workloadDeployment,
	}, input.WaitForDeploymentAvailableInterval...)
}

type AddDeploymentToWorkloadClusterInput struct {
	ClientSet  *kubernetes.Clientset
	Deployment *appsv1.Deployment
	Namespace  string
}

func AddDeploymentToWorkloadCluster(ctx context.Context, input AddDeploymentToWorkloadClusterInput) {
	result, err := input.ClientSet.AppsV1().Deployments(input.Namespace).Create(ctx, input.Deployment, metav1.CreateOptions{})
	Expect(result).NotTo(BeNil())
	Expect(err).To(BeNil(), "nonstop pods need to be successfully deployed")
}

type AddPodDisruptionBudgetInput struct {
	ClientSet *kubernetes.Clientset
	Budget    *v1beta1.PodDisruptionBudget
	Namespace string
}

func AddPodDisruptionBudget(ctx context.Context, input AddPodDisruptionBudgetInput) {
	budget, err := input.ClientSet.PolicyV1beta1().PodDisruptionBudgets(input.Namespace).Create(ctx, input.Budget, metav1.CreateOptions{})
	Expect(budget).NotTo(BeNil())
	Expect(err).To(BeNil(), "podDisruptionBudget needs to be successfully deployed")
}
