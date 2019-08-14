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

package machine

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetNodeReference(t *testing.T) {
	v1alpha2.AddToScheme(scheme.Scheme)
	r := &ReconcileMachine{
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
