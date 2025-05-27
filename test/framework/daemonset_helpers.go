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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	toolscache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	containerutil "sigs.k8s.io/cluster-api/util/container"
)

// WaitForKubeProxyUpgradeInput is the input for WaitForKubeProxyUpgrade.
type WaitForKubeProxyUpgradeInput struct {
	Getter            Getter
	KubernetesVersion string
}

// WaitForKubeProxyUpgrade waits until kube-proxy version matches with the kubernetes version. This is called during KCP upgrade.
func WaitForKubeProxyUpgrade(ctx context.Context, input WaitForKubeProxyUpgradeInput, intervals ...interface{}) {
	By("Ensuring kube-proxy has the correct image")

	// Validate the kube-proxy image according to how KCP sets the image in the kube-proxy DaemonSet.
	// KCP does this based on the default registry of the Kubernetes/kubeadm version.
	wantKubeProxyImage := "registry.k8s.io/kube-proxy:" + containerutil.SemverToOCIImageTag(input.KubernetesVersion)

	Eventually(func() (bool, error) {
		ds := &appsv1.DaemonSet{}

		if err := input.Getter.Get(ctx, client.ObjectKey{Name: "kube-proxy", Namespace: metav1.NamespaceSystem}, ds); err != nil {
			return false, err
		}

		if ds.Spec.Template.Spec.Containers[0].Image == wantKubeProxyImage {
			return true, nil
		}
		return false, nil
	}, intervals...).Should(BeTrue())
}

// WatchDaemonSetLogsByLabelSelectorInput is the input for WatchDaemonSetLogsByLabelSelector.
type WatchDaemonSetLogsByLabelSelectorInput struct {
	GetLister GetLister
	Cache     toolscache.Cache
	ClientSet *kubernetes.Clientset
	Labels    map[string]string
	LogPath   string
}

// WatchDaemonSetLogsByLabelSelector streams logs for all containers for all pods belonging to a daemonset on the basis of label. Each container's logs are streamed
// in a separate goroutine so they can all be streamed concurrently. This only causes a test failure if there are errors
// retrieving the daemonset, its pods, or setting up a log file. If there is an error with the log streaming itself,
// that does not cause the test to fail.
func WatchDaemonSetLogsByLabelSelector(ctx context.Context, input WatchDaemonSetLogsByLabelSelectorInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WatchDaemonSetLogsByLabelSelector")
	Expect(input.Cache).NotTo(BeNil(), "input.Cache is required for WatchDaemonSetLogsByLabelSelector")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for WatchDaemonSetLogsByLabelSelector")
	Expect(input.Labels).NotTo(BeNil(), "input.Selector is required for WatchDaemonSetLogsByLabelSelector")

	daemonSetList := &appsv1.DaemonSetList{}
	Eventually(func() error {
		return input.GetLister.List(ctx, daemonSetList, client.MatchingLabels(input.Labels))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get DaemonSets for labels")

	for _, daemonSet := range daemonSetList.Items {
		watchPodLogs(ctx, watchPodLogsInput{
			Cache:                input.Cache,
			ClientSet:            input.ClientSet,
			Namespace:            daemonSet.Namespace,
			ManagingResourceName: daemonSet.Name,
			LabelSelector:        daemonSet.Spec.Selector,
			LogPath:              input.LogPath,
		})
	}
}
