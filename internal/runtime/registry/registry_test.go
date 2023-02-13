/*
Copyright 2022 The Kubernetes Authors.

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

package registry

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

func TestColdRegistry(t *testing.T) {
	g := NewWithT(t)

	r := New()
	g.Expect(r.IsReady()).To(BeFalse())

	// Add, Remove, List and Get should fail with a cold registry.
	g.Expect(r.Add(&runtimev1.ExtensionConfig{})).ToNot(Succeed())
	g.Expect(r.Remove(&runtimev1.ExtensionConfig{})).ToNot(Succeed())
	_, err := r.List(runtimecatalog.GroupHook{Group: "foo", Hook: "bak"})
	g.Expect(err).To(HaveOccurred())
	_, err = r.Get("foo")
	g.Expect(err).To(HaveOccurred())
}

func TestWarmUpRegistry(t *testing.T) {
	g := NewWithT(t)

	extensionConfigList := &runtimev1.ExtensionConfigList{
		Items: []runtimev1.ExtensionConfig{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-extension",
				},
				Status: runtimev1.ExtensionConfigStatus{
					Handlers: []runtimev1.ExtensionHandler{
						{
							Name: "handler.test-extension",
							RequestHook: runtimev1.GroupVersionHook{
								APIVersion: "foo/v1alpha1",
								Hook:       "bak",
							},
						},
					},
				},
			},
		},
	}

	// WarmUp registry.
	r := New()
	g.Expect(r.WarmUp(extensionConfigList)).To(Succeed())
	g.Expect(r.IsReady()).To(BeTrue())

	// A second WarmUp should fail, registry should stay ready.
	g.Expect(r.WarmUp(extensionConfigList)).ToNot(Succeed())
	g.Expect(r.IsReady()).To(BeTrue())

	// Add, Remove, List and Get should work with a warmed up registry.
	g.Expect(r.Add(&runtimev1.ExtensionConfig{})).To(Succeed())
	g.Expect(r.Remove(&runtimev1.ExtensionConfig{})).To(Succeed())

	registrations, err := r.List(runtimecatalog.GroupHook{Group: "foo", Hook: "bak"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(HaveLen(1))
	g.Expect(registrations[0].Name).To(Equal("handler.test-extension"))

	registration, err := r.Get("handler.test-extension")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registration.Name).To(Equal("handler.test-extension"))
}

func TestRegistry(t *testing.T) {
	g := NewWithT(t)

	extension1 := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "extension1",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extesions1.com/"),
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "foo.extension1",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: "hook.runtime.cluster.x-k8s.io/v1alpha1",
						Hook:       "BeforeClusterUpgrade",
					},
				},
				{
					Name: "bar.extension1",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: "hook.runtime.cluster.x-k8s.io/v1alpha1",
						Hook:       "BeforeClusterUpgrade",
					},
				},
				{
					Name: "baz.extension1",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: "hook.runtime.cluster.x-k8s.io/v1alpha1",
						Hook:       "AfterClusterUpgrade",
					},
				},
			},
		},
	}

	extension2 := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "extension2",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extesions2.com/"),
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "qux.extension2",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: "hook.runtime.cluster.x-k8s.io/v1alpha1",
						Hook:       "AfterClusterUpgrade",
					},
				},
			},
		},
	}

	// WarmUp with extension1
	e := New()
	err := e.WarmUp(&runtimev1.ExtensionConfigList{Items: []runtimev1.ExtensionConfig{*extension1}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(e.IsReady()).To(BeTrue())

	// Get an extension by name
	registration, err := e.Get("foo.extension1")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registration.Name).To(Equal("foo.extension1"))

	// List all BeforeClusterUpgrade extensions
	registrations, err := e.List(runtimecatalog.GroupHook{Group: "hook.runtime.cluster.x-k8s.io", Hook: "BeforeClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(HaveLen(2))
	g.Expect(registrations).To(ContainExtension("foo.extension1"))
	g.Expect(registrations).To(ContainExtension("bar.extension1"))

	// List all AfterClusterUpgrade extensions
	registrations, err = e.List(runtimecatalog.GroupHook{Group: "hook.runtime.cluster.x-k8s.io", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(HaveLen(1))
	g.Expect(registrations).To(ContainExtension("baz.extension1"))

	// Add extension2 with one more AfterClusterUpgrade and check it is there
	g.Expect(e.Add(extension2)).To(Succeed())

	registrations, err = e.List(runtimecatalog.GroupHook{Group: "hook.runtime.cluster.x-k8s.io", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(HaveLen(2))
	g.Expect(registrations).To(ContainExtension("baz.extension1"))
	g.Expect(registrations).To(ContainExtension("qux.extension2"))

	// Remove extension1 and check everything is updated
	g.Expect(e.Remove(extension1)).To(Succeed())

	registrations, err = e.List(runtimecatalog.GroupHook{Group: "hook.runtime.cluster.x-k8s.io", Hook: "BeforeClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(BeEmpty())

	registrations, err = e.List(runtimecatalog.GroupHook{Group: "hook.runtime.cluster.x-k8s.io", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(registrations).To(HaveLen(1))
	g.Expect(registrations).To(ContainExtension("qux.extension2"))
}

func ContainExtension(name string) types.GomegaMatcher {
	return &ContainExtensionMatcher{
		name: name,
	}
}

type ContainExtensionMatcher struct {
	name string
}

func (matcher *ContainExtensionMatcher) Match(actual interface{}) (success bool, err error) {
	ext, ok := actual.([]*ExtensionRegistration)
	if !ok {
		return false, errors.Errorf("Expecting *ExtensionRegistration, got %t", actual)
	}

	for _, e := range ext {
		if e.Name == matcher.name {
			return true, nil
		}
	}
	return false, nil
}

func (matcher *ContainExtensionMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain element matching", matcher.name)
}

func (matcher *ContainExtensionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to contain element matching", matcher.name)
}
