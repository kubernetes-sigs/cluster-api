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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
)

func TestGetNodeReference(t *testing.T) {
	g := NewWithT(t)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	r := &MachineReconciler{
		Client:   fake.NewFakeClientWithScheme(scheme.Scheme),
		Log:      log.Log,
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

	client := fake.NewFakeClientWithScheme(scheme.Scheme, nodeList...)

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
			gt := NewWithT(t)
			providerID, err := noderefutil.NewProviderID(test.providerID)
			gt.Expect(err).NotTo(HaveOccurred(), "Expected no error parsing provider id %q, got %v", test.providerID, err)

			reference, err := r.getNodeReference(client, providerID)
			if test.err == nil {
				g.Expect(err).To(BeNil())
			} else {
				gt.Expect(err).NotTo(BeNil())
				gt.Expect(err).To(Equal(test.err), "Expected error %v, got %v", test.err, err)
			}

			if test.expected == nil && reference == nil {
				return
			}

			gt.Expect(reference.Name).To(Equal(test.expected.Name), "Expected NodeRef's name to be %v, got %v", reference.Name, test.expected.Name)
			gt.Expect(reference.Namespace).To(Equal(test.expected.Namespace), "Expected NodeRef's namespace to be %v, got %v", reference.Namespace, test.expected.Namespace)
		})

	}
}
