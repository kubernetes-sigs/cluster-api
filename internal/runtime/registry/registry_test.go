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
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

func TestColdExtensions(t *testing.T) {
	g := NewWithT(t)

	e := extensions()

	g.Expect(e.IsReady()).To(BeFalse())
	g.Expect(e.Add(&runtimev1.ExtensionConfig{})).ToNot(Succeed())
	g.Expect(e.Remove(&runtimev1.ExtensionConfig{})).ToNot(Succeed())
	_, err := e.List(catalog.GroupVersionHook{Group: "foo", Version: "bar", Hook: "bak"})
	g.Expect(err).To(HaveOccurred())
	_, err = e.Get("foo")
	g.Expect(err).To(HaveOccurred())
}

func TestWarmUpExtensions(t *testing.T) {
	g := NewWithT(t)

	e := extensions()
	err := e.WarmUp(&runtimev1.ExtensionConfigList{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(e.IsReady()).To(BeTrue())

	g.Expect(e.Add(&runtimev1.ExtensionConfig{})).To(Succeed())
	g.Expect(e.Remove(&runtimev1.ExtensionConfig{})).To(Succeed())
	_, err = e.List(catalog.GroupVersionHook{Group: "foo", Version: "bar", Hook: "bak"})
	g.Expect(err).ToNot(HaveOccurred())
	_, err = e.Get("foo")
	g.Expect(err).ToNot(HaveOccurred())
}

func TestExtensions(t *testing.T) {
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

	e := extensions()

	// WarmUp with extension1
	err := e.WarmUp(&runtimev1.ExtensionConfigList{Items: []runtimev1.ExtensionConfig{*extension1}})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(e.IsReady()).To(BeTrue())

	// Get an extension by name
	ex, err := e.Get("foo.extension1")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ex.Name).To(Equal("foo.extension1"))

	// List all the BeforeClusterUpgrade extensions
	ext, err := e.List(catalog.GroupVersionHook{Group: "hook.runtime.cluster.x-k8s.io", Version: "v1alpha1", Hook: "BeforeClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ext).To(HaveLen(2))
	g.Expect(ext).To(ContainExtension("foo.extension1"))
	g.Expect(ext).To(ContainExtension("bar.extension1"))

	// List all the AfterClusterUpgrade extensions
	ext, err = e.List(catalog.GroupVersionHook{Group: "hook.runtime.cluster.x-k8s.io", Version: "v1alpha1", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ext).To(HaveLen(1))
	g.Expect(ext).To(ContainExtension("baz.extension1"))

	// Add extension2 with one more AfterClusterUpgrade and check it is there
	err = e.Add(extension2)
	g.Expect(err).ToNot(HaveOccurred())

	ext, err = e.List(catalog.GroupVersionHook{Group: "hook.runtime.cluster.x-k8s.io", Version: "v1alpha1", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ext).To(HaveLen(2))
	g.Expect(ext).To(ContainExtension("baz.extension1"))
	g.Expect(ext).To(ContainExtension("qux.extension2"))

	// Remove extension1 and check everything is updated
	err = e.Remove(extension1)
	g.Expect(err).ToNot(HaveOccurred())

	ext, err = e.List(catalog.GroupVersionHook{Group: "hook.runtime.cluster.x-k8s.io", Version: "v1alpha1", Hook: "BeforeClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ext).To(HaveLen(0))

	ext, err = e.List(catalog.GroupVersionHook{Group: "hook.runtime.cluster.x-k8s.io", Version: "v1alpha1", Hook: "AfterClusterUpgrade"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ext).To(HaveLen(1))
	g.Expect(ext).To(ContainExtension("qux.extension2"))
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
	ext, ok := actual.([]*RuntimeExtensionRegistration)
	if !ok {
		return false, errors.Errorf("Expecting *RuntimeExtensionRegistration, got %t", actual)
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
