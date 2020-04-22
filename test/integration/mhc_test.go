// +build integration

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

package integration

import (
	"context"
	"flag"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/test/helpers"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

func TestMachineSetsWithMachineHealthCheck(t *testing.T) {
	g := NewWithT(t)
	testEnv, err := helpers.NewTestEnvironment()
	g.Expect(err).NotTo(HaveOccurred())
	defer func() {
		g.Expect(testEnv.Stop()).To(Succeed())
	}()

	klog.InitFlags(nil)
	g.Expect(flag.Set("v", "4")).To(Succeed())
	g.Expect(flag.Set("logtostderr", "false")).To(Succeed())

	logger := klogr.New()

	g.Expect((&controllers.MachineSetReconciler{
		Client: testEnv,
		Log:    logger,
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())
	g.Expect((&controllers.MachineHealthCheckReconciler{
		Client: testEnv,
		Log:    logger,
	}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())

	go func() {
		g.Expect(testEnv.StartManager()).To(Succeed())
	}()

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "mhc-test"}}
	g.Expect(testEnv.Create(context.Background(), namespace)).To(Succeed())
	defer cleanup(g, testEnv, namespace)

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "test-cluster"}}
	g.Expect(testEnv.Create(context.Background(), cluster)).To(Succeed())

	g.Expect(testEnv.CreateKubeconfigSecret(cluster)).To(Succeed())

	bootstrapTmpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapMachineTemplate",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "ms-template",
				"namespace": namespace.Name,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"kind":       "BootstrapMachine",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
					"metadata":   map[string]interface{}{},
				},
			},
		},
	}
	g.Expect(testEnv.Create(context.Background(), bootstrapTmpl)).To(Succeed())

	infraTmpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachineTemplate",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "ms-template",
				"namespace": namespace.Name,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"kind":       "InfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
					"metadata":   map[string]interface{}{},
					"spec": map[string]interface{}{
						"size":       "3xlarge",
						"providerID": "test:////id",
					},
				},
			},
		},
	}
	g.Expect(testEnv.Create(context.Background(), infraTmpl)).To(Succeed())

	instance := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ms-",
			Namespace:    namespace.Name,
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: cluster.Name,
			Replicas:    pointer.Int32Ptr(1),
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label-1": "true",
				},
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						"label-1": "true",
					},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: cluster.Name,
					Version:     pointer.StringPtr("1.0.0"),
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
							Kind:       "BootstrapMachineTemplate",
							Name:       bootstrapTmpl.GetName(),
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureMachineTemplate",
						Name:       infraTmpl.GetName(),
					},
				},
			},
		},
	}
	g.Expect(testEnv.Create(context.Background(), instance)).To(Succeed())

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}
	g.Expect(testEnv.Create(context.Background(), node)).To(Succeed())
	defer cleanup(g, testEnv, node)

	mhc := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mhc",
			Namespace: namespace.Name,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: cluster.Name,
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}
	mhc.Default()
	g.Expect(testEnv.Create(context.Background(), mhc)).To(Succeed())

	var nodes corev1.NodeList
	g.Expect(testEnv.List(context.Background(), &nodes)).To(Succeed())

	var found bool
	for i := range nodes.Items {
		n := nodes.Items[i]
		if n.Name == node.Name {
			found = true
			node = &n
		}
	}
	g.Expect(found).To(BeTrue())

	var machines clusterv1.MachineList
	g.Eventually(func() ([]clusterv1.Machine, error) {
		if err := testEnv.List(context.Background(), &machines); err != nil {
			return nil, err
		}

		return machines.Items, nil
	}, 10*time.Second).Should(HaveLen(1))

	machine := &machines.Items[0]
	machine.Status.NodeRef = &corev1.ObjectReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
		UID:        node.UID,
	}
	failureReason := capierrors.MachineStatusError("foo")
	machine.Status.FailureReason = &failureReason
	g.Expect(testEnv.Status().Update(context.Background(), machine)).To(Succeed())

	machine.ObjectMeta.Finalizers = []string{"whatever"}
	g.Expect(testEnv.Update(context.Background(), machine)).To(Succeed())

	g.Eventually(func() (*metav1.Time, error) {
		if err := testEnv.Get(context.Background(), util.ObjectKey(machine), machine); err != nil {
			return nil, err
		}

		return machine.DeletionTimestamp, nil
	}, 10*time.Second).Should(Not(BeNil()))

	// release finalizer so machine can be deleted
	machine.SetFinalizers(nil)
	g.Expect(testEnv.Update(context.Background(), machine)).To(Succeed())
}

func cleanup(g *WithT, client client.Client, obj runtime.Object) {
	g.Expect(client.Delete(context.Background(), obj)).To(Succeed())
}
