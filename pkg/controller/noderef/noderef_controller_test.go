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

package noderef

import (
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/noderefutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	instance := &v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Machine object and expect the Reconcile
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

func TestGetNodeReference(t *testing.T) {
	v1alpha1.AddToScheme(scheme.Scheme)
	r := &ReconcileNodeRef{
		Client:   fake.NewFakeClient(),
		scheme:   scheme.Scheme,
		recorder: record.NewFakeRecorder(32),
	}

	nodeList := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-east-1/id-node-1",
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "aws://us-west-2/id-node-2",
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gce-node-2",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "gce://us-central1/id-node-2",
			},
		},
	}

	coreV1Client := fakeclient.NewSimpleClientset(nodeList...).CoreV1()

	testCases := []struct {
		name       string
		providerID string
		expected   *corev1.ObjectReference
		err        error
	}{
		{
			name:       "valid provider id, valid aws node",
			providerID: "aws:///id-node-1",
			expected:   &corev1.ObjectReference{Name: "node-1"},
		},
		{
			name:       "valid provider id, valid aws node",
			providerID: "aws:///id-node-2",
			expected:   &corev1.ObjectReference{Name: "node-2"},
		},
		{
			name:       "valid provider id, valid gce node",
			providerID: "gce:///id-node-2",
			expected:   &corev1.ObjectReference{Name: "gce-node-2"},
		},
		{
			name:       "valid provider id, no node found",
			providerID: "aws:///id-node-100",
			expected:   nil,
			err:        ErrNodeNotFound,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			providerID, err := noderefutil.NewProviderID(test.providerID)
			if err != nil {
				t.Fatalf("Expected no error parsing provider id %q, got %v", test.providerID, err)
			}

			reference, err := r.getNodeReference(coreV1Client, providerID)
			if err != nil {
				if (test.err != nil && !strings.Contains(err.Error(), test.err.Error())) || test.err == nil {
					t.Fatalf("Expected error %v, got %v", test.err, err)
				}
			}

			if test.expected == nil && reference == nil {
				return
			}

			if reference.Name != test.expected.Name {
				t.Fatalf("Expected NodeRef's name to be %v, got %v", reference.Name, test.expected.Name)
			}

			if reference.Namespace != test.expected.Namespace {
				t.Fatalf("Expected NodeRef's namespace to be %v, got %v", reference.Namespace, test.expected.Namespace)
			}
		})

	}
}
