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

	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterName      = "test-cluster"
	clusterNamespace = "test-namespace"
)

func TestControlPlaneInitMutex_Lock(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	uid := types.UID("test-uid")

	tests := []struct {
		name          string
		context       context.Context
		client        client.Client
		shouldAcquire bool
	}{
		{
			name:    "should successfully acquire lock if the config cannot be found",
			context: context.Background(),
			client: &fakeClient{
				Client:   fake.NewFakeClientWithScheme(scheme),
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldAcquire: true,
		},
		{
			name:    "should not acquire lock if already exits",
			context: context.Background(),
			client: &fakeClient{
				Client: fake.NewFakeClientWithScheme(scheme, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName(clusterName),
						Namespace: clusterNamespace,
					},
				}),
			},
			shouldAcquire: false,
		},
		{
			name:    "should not acquire lock if cannot create config map",
			context: context.Background(),
			client: &fakeClient{
				Client:      fake.NewFakeClientWithScheme(scheme),
				getError:    apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, configMapName(clusterName)),
				createError: errors.New("create error"),
			},
			shouldAcquire: false,
		},
		{
			name:    "should not acquire lock if config map already exists while creating",
			context: context.Background(),
			client: &fakeClient{
				Client:      fake.NewFakeClientWithScheme(scheme),
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

			gs.Expect(l.Lock(context.Background(), cluster, machine)).To(Equal(tc.shouldAcquire))
		})
	}
}
func TestControlPlaneInitMutex_UnLock(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	uid := types.UID("test-uid")
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(clusterName),
			Namespace: clusterNamespace,
		},
	}
	tests := []struct {
		name          string
		context       context.Context
		client        client.Client
		shouldRelease bool
	}{
		{
			name:    "should release lock by deleting config map",
			context: context.Background(),
			client: &fakeClient{
				Client: fake.NewFakeClientWithScheme(scheme),
			},
			shouldRelease: true,
		},
		{
			name:    "should not release lock if cannot delete config map",
			context: context.Background(),
			client: &fakeClient{
				Client:      fake.NewFakeClientWithScheme(scheme, configMap),
				deleteError: errors.New("delete error"),
			},
			shouldRelease: false,
		},
		{
			name:    "should release lock if config map does not exist",
			context: context.Background(),
			client: &fakeClient{
				Client:   fake.NewFakeClientWithScheme(scheme),
				getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, fmt.Sprintf("%s-controlplane", uid)),
			},
			shouldRelease: true,
		},
		{
			name:    "should not release lock if error while getting config map",
			context: context.Background(),
			client: &fakeClient{
				Client:   fake.NewFakeClientWithScheme(scheme),
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

			gs.Expect(l.Unlock(context.Background(), cluster)).To(Equal(tc.shouldRelease))
		})
	}
}

func TestInfoLines_Lock(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	uid := types.UID("test-uid")
	info := information{MachineName: "my-control-plane"}
	b, err := json.Marshal(info)
	g.Expect(err).NotTo(HaveOccurred())

	c := &fakeClient{
		Client: fake.NewFakeClientWithScheme(scheme, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName(clusterName),
				Namespace: clusterNamespace,
			},
			Data: map[string]string{semaphoreInformationKey: string(b)},
		}),
	}

	logtester := &logtests{
		InfoLog: make([]line, 0),
	}
	l := &ControlPlaneInitMutex{
		log:    logtester,
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

	g.Expect(l.Lock(context.Background(), cluster, machine)).To(BeFalse())

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

func (fc *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	if fc.getError != nil {
		return fc.getError
	}
	return fc.Client.Get(ctx, key, obj)
}

func (fc *fakeClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	if fc.createError != nil {
		return fc.createError
	}
	return fc.Client.Create(ctx, obj, opts...)
}

func (fc *fakeClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
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

func (l *logtests) Info(msg string, keysAndValues ...interface{}) {
	data := make(map[string]interface{})
	for i := 0; i < len(keysAndValues); i += 2 {
		data[keysAndValues[i].(string)] = keysAndValues[i+1]
	}
	l.InfoLog = append(l.InfoLog, line{
		line: msg,
		data: data,
	})
}
func (l *logtests) WithValues(keysAndValues ...interface{}) logr.Logger {
	return l
}
