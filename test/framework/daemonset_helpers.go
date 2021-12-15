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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	Eventually(func() (bool, error) {
		ds := &appsv1.DaemonSet{}

		if err := input.Getter.Get(ctx, client.ObjectKey{Name: "kube-proxy", Namespace: metav1.NamespaceSystem}, ds); err != nil {
			return false, err
		}
		if ds.Spec.Template.Spec.Containers[0].Image == "k8s.gcr.io/kube-proxy:"+containerutil.SemverToOCIImageTag(input.KubernetesVersion) {
			return true, nil
		}
		return false, nil
	}, intervals...).Should(BeTrue())
}
