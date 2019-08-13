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

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	clusterv2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func TestControlPlaneInitLockerAcquire(t *testing.T) {
	tests := []struct {
		name     string
		getError error
	}{
		{
			name:     "create succeeds",
			getError: apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, "uid1-configmap"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &controlPlaneInitLocker{
				log: log.Log,
				configMapClient: &configMapsGetter{
					getError: tc.getError,
				},
			}

			cluster := &clusterv2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "name1",
					UID:       types.UID("uid1"),
				},
			}

			acquired := l.Acquire(cluster)
			if !acquired {
				t.Fatal("acquired was false but it should have been true")
			}
		})
	}
}

func TestControlPlaneInitLockerAcquireErrors(t *testing.T) {
	tests := []struct {
		name          string
		configMap     *v1.ConfigMap
		getError      error
		createError   error
		expectAcquire bool
	}{
		{
			name:          "configmap already exists",
			configMap:     &v1.ConfigMap{},
			expectAcquire: false,
		},
		{
			name:          "error getting configmap",
			getError:      errors.New("get error"),
			expectAcquire: false,
		},
		{
			name:          "create fails",
			getError:      apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, "uid1-configmap"),
			createError:   errors.New("create error"),
			expectAcquire: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &controlPlaneInitLocker{
				log: log.Log,
				configMapClient: &configMapsGetter{
					configMap:   tc.configMap,
					getError:    tc.getError,
					createError: tc.createError,
				},
			}

			cluster := &clusterv2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "name1",
					UID:       types.UID("uid1"),
				},
			}

			acquired := l.Acquire(cluster)
			if acquired {
				t.Fatal("expected acquired to be false but it is true")
			}
		})
	}
}

func TestControlPlaneInitLockerRelease(t *testing.T) {
	tests := []struct {
		name          string
		configMap     *v1.ConfigMap
		getError      error
		deleteError   error
		expectRelease bool
	}{
		{
			name:          "error getting configmap",
			getError:      errors.New("get error"),
			expectRelease: false,
		},
		{
			name:          "configmap not found",
			getError:      apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, "uid1-configmap"),
			expectRelease: true,
		},
		{
			name:          "delete succeeds",
			expectRelease: true,
		},
		{
			name:          "delete fails",
			deleteError:   errors.New("delete error"),
			expectRelease: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &controlPlaneInitLocker{
				log: log.Log,
				configMapClient: &configMapsGetter{
					configMap:   tc.configMap,
					getError:    tc.getError,
					deleteError: tc.deleteError,
				},
			}

			cluster := &clusterv2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "name1",
					UID:       types.UID("uid1"),
				},
			}

			released := l.Release(cluster)
			if tc.expectRelease != released {
				t.Errorf("expected %t, got %t", tc.expectRelease, released)
			}
		})
	}
}

type configMapsGetter struct {
	configMap   *v1.ConfigMap
	getError    error
	createError error
	deleteError error
}

func (c *configMapsGetter) ConfigMaps(namespace string) corev1client.ConfigMapInterface {
	return &configMapClient{
		configMap:   c.configMap,
		getError:    c.getError,
		createError: c.createError,
		deleteError: c.deleteError,
	}
}

type configMapClient struct {
	configMap   *v1.ConfigMap
	getError    error
	createError error
	deleteError error
}

func (c *configMapClient) Create(configMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	return c.configMap, c.createError
}

func (c *configMapClient) Get(name string, getOptions metav1.GetOptions) (*v1.ConfigMap, error) {
	if c.getError != nil {
		return nil, c.getError
	}
	return c.configMap, nil
}

func (c *configMapClient) Update(*v1.ConfigMap) (*v1.ConfigMap, error) {
	panic("not implemented")
}

func (c *configMapClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c.deleteError
}

func (c *configMapClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	panic("not implemented")
}

func (c *configMapClient) List(opts metav1.ListOptions) (*v1.ConfigMapList, error) {
	panic("not implemented")
}

func (c *configMapClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *configMapClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ConfigMap, err error) {
	panic("not implemented")
}
