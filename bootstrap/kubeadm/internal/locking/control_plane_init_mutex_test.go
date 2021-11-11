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

package locking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterName      = "test-cluster"
	clusterNamespace = "test-namespace"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func TestControlPlaneInitMutex_Lock(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	uid := types.UID("test-uid")

	tests := []struct {
		name          string
		client        client.Client
		shouldAcquire bool
	}{
		{
			name: "should successfully acquire lock if the config cannot be found",
			client: &fakeClient{
				Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldAcquire: true,
		},
		{
			name: "should not acquire lock if already exits",
			client: &fakeClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName(clusterName),
						Namespace: clusterNamespace,
					},
				}).Build(),
			},
			shouldAcquire: false,
		},
		{
			name: "should not acquire lock if cannot create config map",
			client: &fakeClient{
				Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
				getError:    apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, configMapName(clusterName)),
				createError: errors.New("create error"),
			},
			shouldAcquire: false,
		},
		{
			name: "should not acquire lock if config map already exists while creating",
			client: &fakeClient{
				Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
				getError:    apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
				createError: apierrors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldAcquire: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			l := &ControlPlaneInitMutex{
				log:    log.Log,
				client: tc.client,
			}

			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: clusterNamespace,
					Name:      clusterName,
					UID:       uid,
				},
			}
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("machine-%s", cluster.Name),
				},
			}

			gs.Expect(l.Lock(ctx, cluster, machine)).To(Equal(tc.shouldAcquire))
		})
	}
}
func TestControlPlaneInitMutex_UnLock(t *testing.T) {
	uid := types.UID("test-uid")
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(clusterName),
			Namespace: clusterNamespace,
		},
	}
	tests := []struct {
		name          string
		client        client.Client
		shouldRelease bool
	}{
		{
			name: "should release lock by deleting config map",
			client: &fakeClient{
				Client: fake.NewClientBuilder().Build(),
			},
			shouldRelease: true,
		},
		{
			name: "should not release lock if cannot delete config map",
			client: &fakeClient{
				Client:      fake.NewClientBuilder().WithObjects(configMap).Build(),
				deleteError: errors.New("delete error"),
			},
			shouldRelease: false,
		},
		{
			name: "should release lock if config map does not exist",
			client: &fakeClient{
				Client:   fake.NewClientBuilder().Build(),
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldRelease: true,
		},
		{
			name: "should not release lock if error while getting config map",
			client: &fakeClient{
				Client:   fake.NewClientBuilder().Build(),
				getError: errors.New("get error"),
			},
			shouldRelease: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			l := &ControlPlaneInitMutex{
				log:    log.Log,
				client: tc.client,
			}

			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: clusterNamespace,
					Name:      clusterName,
					UID:       uid,
				},
			}

			gs.Expect(l.Unlock(ctx, cluster)).To(Equal(tc.shouldRelease))
		})
	}
}

func TestInfoLines_Lock(t *testing.T) {
	g := NewWithT(t)

	uid := types.UID("test-uid")
	info := information{MachineName: "my-control-plane"}
	b, err := json.Marshal(info)
	g.Expect(err).NotTo(HaveOccurred())

	c := &fakeClient{
		Client: fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName(clusterName),
				Namespace: clusterNamespace,
			},
			Data: map[string]string{semaphoreInformationKey: string(b)},
		}).Build(),
	}

	logtester := &logtests{
		InfoLog: make([]line, 0),
	}
	l := &ControlPlaneInitMutex{
		log:    logr.New(logtester),
		client: c,
	}

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterNamespace,
			Name:      clusterName,
			UID:       uid,
		},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("machine-%s", cluster.Name),
		},
	}

	g.Expect(l.Lock(ctx, cluster, machine)).To(BeFalse())

	foundLogLine := false
	for _, line := range logtester.InfoLog {
		for k, v := range line.data {
			if k == "init-machine" && v.(string) == "my-control-plane" {
				foundLogLine = true
			}
		}
	}

	g.Expect(foundLogLine).To(BeTrue())
}

type fakeClient struct {
	client.Client
	getError    error
	createError error
	deleteError error
}

func (fc *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if fc.getError != nil {
		return fc.getError
	}
	return fc.Client.Get(ctx, key, obj)
}

func (fc *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if fc.createError != nil {
		return fc.createError
	}
	return fc.Client.Create(ctx, obj, opts...)
}

func (fc *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if fc.deleteError != nil {
		return fc.deleteError
	}
	return fc.Client.Delete(ctx, obj, opts...)
}

type logtests struct {
	logr.Logger
	InfoLog []line
}

type line struct {
	line string
	data map[string]interface{}
}

func (l *logtests) Init(info logr.RuntimeInfo) {
}

func (l *logtests) Enabled(level int) bool {
	return true
}

func (l *logtests) Info(level int, msg string, keysAndValues ...interface{}) {
	data := make(map[string]interface{})
	for i := 0; i < len(keysAndValues); i += 2 {
		data[keysAndValues[i].(string)] = keysAndValues[i+1]
	}
	l.InfoLog = append(l.InfoLog, line{
		line: msg,
		data: data,
	})
}
func (l *logtests) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return l
}

func (l *logtests) WithName(name string) logr.LogSink {
	return l
}
