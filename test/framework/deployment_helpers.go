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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/prometheus/common/expfmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	toolscache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

const (
	nodeRoleControlPlane = "node-role.kubernetes.io/control-plane"
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
	Byf("Waiting for deployment %s to be available", klog.KObj(input.Deployment))
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
	b.WriteString(fmt.Sprintf("Deployment %s failed to get status.Available = True condition",
		klog.KObj(input.Deployment)))
	if deployment == nil {
		b.WriteString("\nDeployment: nil\n")
	} else {
		b.WriteString(fmt.Sprintf("\nDeployment:\n%s\n", PrettyPrint(deployment)))
	}
	return b.String()
}

// WatchDeploymentLogsByLabelSelectorInput is the input for WatchDeploymentLogsByLabelSelector.
type WatchDeploymentLogsByLabelSelectorInput struct {
	GetLister GetLister
	Cache     toolscache.Cache
	ClientSet *kubernetes.Clientset
	Labels    map[string]string
	LogPath   string
}

// WatchDeploymentLogsByLabelSelector streams logs for all containers for all pods belonging to a deployment on the basis of label. Each container's logs are streamed
// in a separate goroutine so they can all be streamed concurrently. This only causes a test failure if there are errors
// retrieving the deployment, its pods, or setting up a log file. If there is an error with the log streaming itself,
// that does not cause the test to fail.
func WatchDeploymentLogsByLabelSelector(ctx context.Context, input WatchDeploymentLogsByLabelSelectorInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WatchDeploymentLogsByLabelSelector")
	Expect(input.Cache).NotTo(BeNil(), "input.Cache is required for WatchDeploymentLogsByLabelSelector")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for WatchDeploymentLogsByLabelSelector")
	Expect(input.Labels).NotTo(BeNil(), "input.Selector is required for WatchDeploymentLogsByLabelSelector")

	deploymentList := &appsv1.DeploymentList{}
	Eventually(func() error {
		return input.GetLister.List(ctx, deploymentList, client.MatchingLabels(input.Labels))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get deployment for labels")

	for _, deployment := range deploymentList.Items {
		watchPodLogs(ctx, watchPodLogsInput{
			Cache:                input.Cache,
			ClientSet:            input.ClientSet,
			Namespace:            deployment.Namespace,
			ManagingResourceName: deployment.Name,
			LabelSelector:        deployment.Spec.Selector,
			LogPath:              input.LogPath,
		})
	}
}

// WatchDeploymentLogsByNameInput is the input for WatchDeploymentLogsByName.
type WatchDeploymentLogsByNameInput struct {
	GetLister  GetLister
	Cache      toolscache.Cache
	ClientSet  *kubernetes.Clientset
	Deployment *appsv1.Deployment
	LogPath    string
}

// WatchDeploymentLogsByName streams logs for all containers for all pods belonging to a deployment. Each container's logs are streamed
// in a separate goroutine so they can all be streamed concurrently. This only causes a test failure if there are errors
// retrieving the deployment, its pods, or setting up a log file. If there is an error with the log streaming itself,
// that does not cause the test to fail.
func WatchDeploymentLogsByName(ctx context.Context, input WatchDeploymentLogsByNameInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WatchDeploymentLogsByName")
	Expect(input.Cache).NotTo(BeNil(), "input.Cache is required for WatchDeploymentLogsByName")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for WatchDeploymentLogsByName")
	Expect(input.Deployment).NotTo(BeNil(), "input.Deployment is required for WatchDeploymentLogsByName")

	deployment := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(input.Deployment)
	Eventually(func() error {
		return input.GetLister.Get(ctx, key, deployment)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get deployment %s", klog.KObj(input.Deployment))

	watchPodLogs(ctx, watchPodLogsInput{
		Cache:                input.Cache,
		ClientSet:            input.ClientSet,
		Namespace:            deployment.Namespace,
		ManagingResourceName: deployment.Name,
		LabelSelector:        deployment.Spec.Selector,
		LogPath:              input.LogPath,
	})
}

// watchPodLogsInput is the input for watchPodLogs.
type watchPodLogsInput struct {
	Cache                toolscache.Cache
	ClientSet            *kubernetes.Clientset
	Namespace            string
	ManagingResourceName string
	LabelSelector        *metav1.LabelSelector
	LogPath              string
}

// watchPodLogs streams logs for all containers for all pods belonging to a deployment with the given label. Each container's logs are streamed
// in a separate goroutine so they can all be streamed concurrently. This only causes a test failure if there are errors
// retrieving the deployment, its pods, or setting up a log file. If there is an error with the log streaming itself,
// that does not cause the test to fail.
func watchPodLogs(ctx context.Context, input watchPodLogsInput) {
	// Create informer to watch for pods matching input.

	podInformer, err := input.Cache.GetInformer(ctx, &corev1.Pod{})
	Expect(err).ToNot(HaveOccurred(), "Failed to create controller-runtime informer from cache")

	selector, err := metav1.LabelSelectorAsSelector(input.LabelSelector)
	Expect(err).ToNot(HaveOccurred())

	eventHandler := newWatchPodLogsEventHandler(ctx, input, selector)

	handlerRegistration, err := podInformer.AddEventHandler(eventHandler)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		<-ctx.Done()
		Expect(podInformer.RemoveEventHandler(handlerRegistration)).To(Succeed())
	}()
}

type watchPodLogsEventHandler struct {
	//nolint:containedctx
	ctx         context.Context
	input       watchPodLogsInput
	selector    labels.Selector
	startedPods sync.Map
}

func newWatchPodLogsEventHandler(ctx context.Context, input watchPodLogsInput, selector labels.Selector) cache.ResourceEventHandler {
	return &watchPodLogsEventHandler{
		ctx:         ctx,
		input:       input,
		selector:    selector,
		startedPods: sync.Map{},
	}
}

func (eh *watchPodLogsEventHandler) OnAdd(obj interface{}, _ bool) {
	pod := obj.(*corev1.Pod)
	eh.streamPodLogs(pod)
}

func (eh *watchPodLogsEventHandler) OnUpdate(_, newObj interface{}) {
	pod := newObj.(*corev1.Pod)
	eh.streamPodLogs(pod)
}

func (eh *watchPodLogsEventHandler) OnDelete(_ interface{}) {}

func (eh *watchPodLogsEventHandler) streamPodLogs(pod *corev1.Pod) {
	if pod.GetNamespace() != eh.input.Namespace {
		return
	}
	if !eh.selector.Matches(labels.Set(pod.GetLabels())) {
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		return
	}
	if _, loaded := eh.startedPods.LoadOrStore(pod.GetUID(), struct{}{}); loaded {
		return
	}

	for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
		log.Logf("Creating log watcher for controller %s, pod %s, container %s", klog.KRef(eh.input.Namespace, eh.input.ManagingResourceName), pod.Name, container.Name)

		// Create log metadata file.
		logMetadataFile := filepath.Clean(path.Join(eh.input.LogPath, eh.input.ManagingResourceName, pod.Name, container.Name+"-log-metadata.json"))
		Expect(os.MkdirAll(filepath.Dir(logMetadataFile), 0750)).To(Succeed())

		metadata := logMetadata{
			Job:       eh.input.Namespace + "/" + eh.input.ManagingResourceName,
			Namespace: eh.input.Namespace,
			App:       eh.input.ManagingResourceName,
			Pod:       pod.Name,
			Container: container.Name,
			NodeName:  pod.Spec.NodeName,
			Stream:    "stderr",
		}
		metadataBytes, err := json.Marshal(&metadata)
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(logMetadataFile, metadataBytes, 0600)).To(Succeed())

		// Watch each container's logs in a goroutine so we can stream them all concurrently.
		go func(pod *corev1.Pod, container corev1.Container) {
			defer GinkgoRecover()

			logFile := filepath.Clean(path.Join(eh.input.LogPath, eh.input.ManagingResourceName, pod.Name, container.Name+".log"))
			Expect(os.MkdirAll(filepath.Dir(logFile), 0750)).To(Succeed())

			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			Expect(err).ToNot(HaveOccurred())
			defer f.Close()

			opts := &corev1.PodLogOptions{
				Container: container.Name,
				Follow:    true,
			}

			// Retry streaming the logs of the pods unless ctx.Done() or if the pod does not exist anymore.
			err = wait.PollUntilContextCancel(eh.ctx, 2*time.Second, false, func(ctx context.Context) (done bool, err error) {
				// Wait for pod to be in running state
				actual, err := eh.input.ClientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
				if err != nil {
					// The pod got deleted if the error IsNotFound. In this case there are also no logs to stream anymore.
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					// Only log the error to not cause the test to fail via GinkgoRecover
					log.Logf("Error getting pod %s, container %s: %v", klog.KRef(pod.Namespace, pod.Name), container.Name, err)
					return true, nil
				}
				// Retry later if pod is currently not running
				if actual.Status.Phase != corev1.PodRunning {
					return false, nil
				}
				podLogs, err := eh.input.ClientSet.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts).Stream(ctx)
				if err != nil {
					// Only log the error to not cause the test to fail via GinkgoRecover
					log.Logf("Error starting logs stream for pod %s, container %s: %v", klog.KRef(pod.Namespace, pod.Name), container.Name, err)
					return true, nil
				}
				defer podLogs.Close()

				out := bufio.NewWriter(f)
				defer out.Flush()
				_, err = out.ReadFrom(podLogs)
				if err != nil && err != io.ErrUnexpectedEOF {
					// Failing to stream logs should not cause the test to fail
					log.Logf("Got error while streaming logs for pod %s, container %s: %v", klog.KRef(pod.Namespace, pod.Name), container.Name, err)
				}
				return false, nil
			})
			if err != nil {
				log.Logf("Stopped streaming logs for pod %s, container %s: %v", klog.KRef(pod.Namespace, pod.Name), container.Name, err)
			}
		}(pod, container)
	}
}

// logMetadata contains metadata about the logs.
// The format is very similar to the one used by promtail.
type logMetadata struct {
	Job       string            `json:"job"`
	Namespace string            `json:"namespace"`
	App       string            `json:"app"`
	Pod       string            `json:"pod"`
	Container string            `json:"container"`
	NodeName  string            `json:"node_name"`
	Stream    string            `json:"stream"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type WatchPodMetricsInput struct {
	GetLister   GetLister
	ClientSet   *kubernetes.Clientset
	Deployment  *appsv1.Deployment
	MetricsPath string
}

// WatchPodMetrics captures metrics from all pods every 5s. It expects to find port 8080 open on the controller.
func WatchPodMetrics(ctx context.Context, input WatchPodMetricsInput) {
	// Dump metrics periodically.
	ticker := time.NewTicker(time.Second * 10)
	Expect(ctx).NotTo(BeNil(), "ctx is required for dumpContainerMetrics")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for dumpContainerMetrics")
	Expect(input.Deployment).NotTo(BeNil(), "input.Deployment is required for dumpContainerMetrics")

	deployment := &appsv1.Deployment{}
	key := client.ObjectKeyFromObject(input.Deployment)
	Eventually(func() error {
		return input.GetLister.Get(ctx, key, deployment)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get deployment %s", klog.KObj(input.Deployment))

	selector, err := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	Expect(err).NotTo(HaveOccurred(), "Failed to create Pods selector for deployment %s", klog.KObj(input.Deployment))

	pods := &corev1.PodList{}
	Eventually(func() error {
		return input.GetLister.List(ctx, pods, client.InNamespace(input.Deployment.Namespace), client.MatchingLabels(selector))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list Pods for deployment %s", klog.KObj(input.Deployment))

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

		var errorRetrievingMetrics bool
		if err != nil {
			// Failing to dump metrics should not cause the test to fail
			errorRetrievingMetrics = true
			data = []byte(fmt.Sprintf("Error retrieving metrics for pod %s: %v\n%s", klog.KRef(pod.Namespace, pod.Name), err, string(data)))
			metricsFile = path.Join(metricsDir, "metrics-error.txt")
		}

		if err := os.WriteFile(metricsFile, data, 0600); err != nil {
			// Failing to dump metrics should not cause the test to fail
			log.Logf("Error writing metrics for pod %s: %v", klog.KRef(pod.Namespace, pod.Name), err)
		}

		if !errorRetrievingMetrics {
			Expect(verifyMetrics(data)).To(Succeed())
		}
	}
}

func verifyMetrics(data []byte) error {
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return errors.Wrapf(err, "failed to parse data to metrics families")
	}

	var errs []error
	for metric, metricFamily := range mf {
		if metric == "controller_runtime_reconcile_panics_total" {
			for _, controllerPanicMetric := range metricFamily.Metric {
				if controllerPanicMetric.Counter != nil && controllerPanicMetric.Counter.Value != nil && *controllerPanicMetric.Counter.Value > 0 {
					controllerName := "unknown"
					for _, label := range controllerPanicMetric.Label {
						if *label.Name == "controller" {
							controllerName = *label.Value
						}
					}
					errs = append(errs, fmt.Errorf("%.0f panics occurred in %q controller (check logs for more details)", *controllerPanicMetric.Counter.Value, controllerName))
				}
			}
		}

		if metric == "controller_runtime_webhook_panics_total" {
			for _, webhookPanicMetric := range metricFamily.Metric {
				if webhookPanicMetric.Counter != nil && webhookPanicMetric.Counter.Value != nil && *webhookPanicMetric.Counter.Value > 0 {
					errs = append(errs, fmt.Errorf("%.0f panics occurred in webhooks (check logs for more details)", *webhookPanicMetric.Counter.Value))
				}
			}
		}
	}

	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}

	return nil
}

// WaitForDNSUpgradeInput is the input for WaitForDNSUpgrade.
type WaitForDNSUpgradeInput struct {
	Getter     Getter
	DNSVersion string
}

// WaitForDNSUpgrade waits until CoreDNS version matches with the CoreDNS upgrade version and all its replicas
// are ready for use with the upgraded version. This is called during KCP upgrade.
func WaitForDNSUpgrade(ctx context.Context, input WaitForDNSUpgradeInput, intervals ...interface{}) {
	By("Ensuring CoreDNS has the correct image")

	Eventually(func() (bool, error) {
		d := &appsv1.Deployment{}

		if err := input.Getter.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, d); err != nil {
			return false, err
		}

		// NOTE: coredns image name has changed over time (k8s.gcr.io/coredns,
		// k8s.gcr.io/coredns/coredns), so we are checking if the version actually changed.
		if strings.HasSuffix(d.Spec.Template.Spec.Containers[0].Image, fmt.Sprintf(":%s", input.DNSVersion)) &&
			// Also check whether the upgraded CoreDNS replicas are available and ready for use.
			d.Status.ObservedGeneration >= d.Generation &&
			d.Spec.Replicas != nil && d.Status.UpdatedReplicas == *d.Spec.Replicas && d.Status.AvailableReplicas == *d.Spec.Replicas {
			return true, nil
		}

		return false, nil
	}, intervals...).Should(BeTrue())
}

type DeployPodAndWaitInput struct {
	WorkloadClusterProxy ClusterProxy
	ControlPlane         *controlplanev1.KubeadmControlPlane
	MachineDeployment    *clusterv1.MachineDeployment
	DeploymentName       string
	Namespace            string
	NodeSelector         map[string]string

	ModifyDeployment func(deployment *appsv1.Deployment)

	WaitForDeploymentAvailableInterval []interface{}
}

// DeployUnevictablePod will deploy a Deployment on a ControlPlane or MachineDeployment.
// It will deploy one Pod replica to each Machine and then deploy a PDB to ensure none of the Pods can be evicted.
func DeployUnevictablePod(ctx context.Context, input DeployPodAndWaitInput) {
	DeployPodAndWait(ctx, input)

	budget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.DeploymentName,
			Namespace: input.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":        "nonstop",
					"deployment": input.DeploymentName,
				},
			},
			// Setting MaxUnavailable to 0 means no Pods can be evicted / unavailable.
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 0,
			},
		},
	}

	AddPodDisruptionBudget(ctx, AddPodDisruptionBudgetInput{
		Namespace: input.Namespace,
		ClientSet: input.WorkloadClusterProxy.GetClientSet(),
		Budget:    budget,
	})
}

// DeployPodAndWait will deploy a Deployment on a ControlPlane or MachineDeployment.
func DeployPodAndWait(ctx context.Context, input DeployPodAndWaitInput) {
	Expect(input.DeploymentName).ToNot(BeNil(), "Need a deployment name in DeployPodAndWait")
	Expect(input.Namespace).ToNot(BeNil(), "Need a namespace in DeployPodAndWait")
	Expect(input.WorkloadClusterProxy).ToNot(BeNil(), "Need a workloadClusterProxy in DeployPodAndWait")
	Expect((input.MachineDeployment == nil && input.ControlPlane != nil) ||
		(input.MachineDeployment != nil && input.ControlPlane == nil)).To(BeTrue(), "Either MachineDeployment or ControlPlane must be set in DeployPodAndWait")

	EnsureNamespace(ctx, input.WorkloadClusterProxy.GetClient(), input.Namespace)

	workloadDeployment := generateDeployment(generateDeploymentInput{
		ControlPlane:      input.ControlPlane,
		MachineDeployment: input.MachineDeployment,
		Name:              input.DeploymentName,
		Namespace:         input.Namespace,
		NodeSelector:      input.NodeSelector,
	})

	input.ModifyDeployment(workloadDeployment)

	AddDeploymentToWorkloadCluster(ctx, AddDeploymentToWorkloadClusterInput{
		Namespace:  input.Namespace,
		ClientSet:  input.WorkloadClusterProxy.GetClientSet(),
		Deployment: workloadDeployment,
	})

	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.WorkloadClusterProxy.GetClient(),
		Deployment: workloadDeployment,
	}, input.WaitForDeploymentAvailableInterval...)
}

type DeployEvictablePodInput struct {
	WorkloadClusterProxy ClusterProxy
	ControlPlane         *controlplanev1.KubeadmControlPlane
	MachineDeployment    *clusterv1.MachineDeployment
	DeploymentName       string
	Namespace            string
	NodeSelector         map[string]string

	ModifyDeployment func(deployment *appsv1.Deployment)

	WaitForDeploymentAvailableInterval []interface{}
}

// DeployEvictablePod will deploy a Deployment on a ControlPlane or MachineDeployment.
// It will deploy one Pod replica to each Machine.
func DeployEvictablePod(ctx context.Context, input DeployEvictablePodInput) {
	Expect(input.DeploymentName).ToNot(BeNil(), "Need a deployment name in DeployEvictablePod")
	Expect(input.Namespace).ToNot(BeNil(), "Need a namespace in DeployEvictablePod")
	Expect(input.WorkloadClusterProxy).ToNot(BeNil(), "Need a workloadClusterProxy in DeployEvictablePod")
	Expect((input.MachineDeployment == nil && input.ControlPlane != nil) ||
		(input.MachineDeployment != nil && input.ControlPlane == nil)).To(BeTrue(), "Either MachineDeployment or ControlPlane must be set in DeployEvictablePod")

	EnsureNamespace(ctx, input.WorkloadClusterProxy.GetClient(), input.Namespace)

	workloadDeployment := generateDeployment(generateDeploymentInput{
		ControlPlane:      input.ControlPlane,
		MachineDeployment: input.MachineDeployment,
		Name:              input.DeploymentName,
		Namespace:         input.Namespace,
		NodeSelector:      input.NodeSelector,
	})

	input.ModifyDeployment(workloadDeployment)

	workloadClient := input.WorkloadClusterProxy.GetClientSet()

	AddDeploymentToWorkloadCluster(ctx, AddDeploymentToWorkloadClusterInput{
		Namespace:  input.Namespace,
		ClientSet:  workloadClient,
		Deployment: workloadDeployment,
	})

	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.WorkloadClusterProxy.GetClient(),
		Deployment: workloadDeployment,
	}, input.WaitForDeploymentAvailableInterval...)
}

type generateDeploymentInput struct {
	ControlPlane      *controlplanev1.KubeadmControlPlane
	MachineDeployment *clusterv1.MachineDeployment
	Name              string
	Namespace         string
	NodeSelector      map[string]string
}

func generateDeployment(input generateDeploymentInput) *appsv1.Deployment {
	workloadDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: input.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":        "nonstop",
					"deployment": input.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":        "nonstop",
						"deployment": input.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "registry.k8s.io/pause:3.10",
						},
					},
					Affinity: &corev1.Affinity{
						// Make sure only 1 Pod of this Deployment can run on the same Node.
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "deployment",
												Operator: "In",
												Values:   []string{input.Name},
											},
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				},
			},
		},
	}

	if input.ControlPlane != nil {
		workloadDeployment.Spec.Template.Spec.NodeSelector = map[string]string{nodeRoleControlPlane: ""}
		workloadDeployment.Spec.Template.Spec.Tolerations = []corev1.Toleration{
			{
				Key:    nodeRoleControlPlane,
				Effect: "NoSchedule",
			},
		}
		workloadDeployment.Spec.Replicas = input.ControlPlane.Spec.Replicas
	}
	if input.MachineDeployment != nil {
		workloadDeployment.Spec.Replicas = input.MachineDeployment.Spec.Replicas
	}

	// Note: If set, the NodeSelector field overwrites the NodeSelector we set above for control plane nodes.
	if input.NodeSelector != nil {
		workloadDeployment.Spec.Template.Spec.NodeSelector = input.NodeSelector
	}

	return workloadDeployment
}

type AddDeploymentToWorkloadClusterInput struct {
	ClientSet  *kubernetes.Clientset
	Deployment *appsv1.Deployment
	Namespace  string
}

func AddDeploymentToWorkloadCluster(ctx context.Context, input AddDeploymentToWorkloadClusterInput) {
	Eventually(func() error {
		result, err := input.ClientSet.AppsV1().Deployments(input.Namespace).Create(ctx, input.Deployment, metav1.CreateOptions{})
		if result != nil && err == nil {
			return nil
		}
		return fmt.Errorf("deployment %s not successfully created in workload cluster: %v", klog.KObj(input.Deployment), err)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create deployment %s in workload cluster", klog.KObj(input.Deployment))
}

type AddPodDisruptionBudgetInput struct {
	ClientSet *kubernetes.Clientset
	Budget    *policyv1.PodDisruptionBudget
	Namespace string
}

func AddPodDisruptionBudget(ctx context.Context, input AddPodDisruptionBudgetInput) {
	Eventually(func() error {
		budget, err := input.ClientSet.PolicyV1().PodDisruptionBudgets(input.Namespace).Create(ctx, input.Budget, metav1.CreateOptions{})
		if budget != nil && err == nil {
			return nil
		}
		return fmt.Errorf("podDisruptionBudget needs to be successfully deployed: %v", err)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "podDisruptionBudget needs to be successfully deployed")
}

type DeletePodDisruptionBudgetInput struct {
	ClientSet *kubernetes.Clientset
	Budget    string
	Namespace string
}

func DeletePodDisruptionBudget(ctx context.Context, input DeletePodDisruptionBudgetInput) {
	Eventually(func() error {
		err := input.ClientSet.PolicyV1().PodDisruptionBudgets(input.Namespace).Delete(ctx, input.Budget, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) || err == nil {
			return nil
		}
		return fmt.Errorf("podDisruptionBudget needs to be deleted: %v", err)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "podDisruptionBudget needs to be deleted")
}
