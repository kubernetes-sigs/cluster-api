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

package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/version"
)

var _ Proxy = &test.FakeProxy{}

func TestProxyGetConfig(t *testing.T) {
	t.Run("GetConfig", func(t *testing.T) {
		tests := []struct {
			name               string
			context            string
			kubeconfigContents string
			expectedHost       string
			expectErr          bool
		}{
			{
				name:               "defaults to the currentContext if none is specified",
				kubeconfigContents: kubeconfig("management", "default"),
				expectedHost:       "https://management-server:1234",
				expectErr:          false,
			},
			{
				name:               "returns host of cluster associated with the specified context even if currentContext is different",
				kubeconfigContents: kubeconfig("management", "default"),
				context:            "workload",
				expectedHost:       "https://kind-server:38790",
				expectErr:          false,
			},
			{
				name:               "returns error if cannot load the kubeconfig file",
				expectErr:          true,
				kubeconfigContents: "bad contents",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				dir, err := os.MkdirTemp("", "clusterctl")
				g.Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(dir)
				configFile := filepath.Join(dir, ".test-kubeconfig.yaml")
				g.Expect(os.WriteFile(configFile, []byte(tt.kubeconfigContents), 0600)).To(Succeed())

				proxy := newProxy(Kubeconfig{Path: configFile, Context: tt.context})
				conf, err := proxy.GetConfig()
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(conf).ToNot(BeNil())
				// asserting on the host of the cluster associated with the
				// context
				g.Expect(conf.Host).To(Equal(tt.expectedHost))
				g.Expect(conf.UserAgent).To(Equal(fmt.Sprintf("clusterctl/%s (%s)", version.Get().GitVersion, version.Get().Platform)))
				g.Expect(conf.QPS).To(BeEquivalentTo(20))
				g.Expect(conf.Burst).To(BeEquivalentTo(100))
				g.Expect(conf.Timeout.String()).To(Equal("30s"))
			})
		}
	})

	t.Run("configure timeout", func(t *testing.T) {
		g := NewWithT(t)
		dir, err := os.MkdirTemp("", "clusterctl")
		g.Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir)
		configFile := filepath.Join(dir, ".test-kubeconfig.yaml")
		g.Expect(os.WriteFile(configFile, []byte(kubeconfig("management", "default")), 0600)).To(Succeed())

		proxy := newProxy(Kubeconfig{Path: configFile, Context: "management"}, InjectProxyTimeout(23*time.Second))
		conf, err := proxy.GetConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(conf.Timeout.String()).To(Equal("23s"))
	})
}

// These tests are emulating the files passed in via KUBECONFIG env var by
// injecting the file paths into the ClientConfigLoadingRules.Precedence
// chain.
func TestKUBECONFIGEnvVar(t *testing.T) {
	t.Run("CurrentNamespace", func(t *testing.T) {
		// KUBECONFIG can specify multiple config files. We should be able to
		// get the correct namespace for the context by parsing through all
		// kubeconfig files
		var (
			context            = "workload"
			kubeconfigContents = kubeconfig("does-not-exist", "some-ns")
		)

		g := NewWithT(t)
		dir, err := os.MkdirTemp("", "clusterctl")
		g.Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir)
		configFile := filepath.Join(dir, ".test-kubeconfig.yaml")
		g.Expect(os.WriteFile(configFile, []byte(kubeconfigContents), 0600)).To(Succeed())

		proxy := newProxy(
			// dont't give an explicit path but rather define the file in the
			// configLoadingRules precedence chain.
			Kubeconfig{Path: "", Context: context},
			InjectKubeconfigPaths([]string{configFile}),
		)
		actualNS, err := proxy.CurrentNamespace()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actualNS).To(Equal("some-ns"))
	})

	t.Run("GetConfig", func(t *testing.T) {
		// KUBECONFIG can specify multiple config files. We should be able to
		// get the valid cluster context by parsing all the kubeconfig files.
		var (
			context = "workload"
			// TODO: If we change current context to "do-not-exist", we get an
			// error. See https://github.com/kubernetes/client-go/issues/797
			kubeconfigContents = kubeconfig("management", "default")
			expectedHost       = "https://kind-server:38790"
		)
		g := NewWithT(t)
		dir, err := os.MkdirTemp("", "clusterctl")
		g.Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir)
		configFile := filepath.Join(dir, ".test-kubeconfig.yaml")
		g.Expect(os.WriteFile(configFile, []byte(kubeconfigContents), 0600)).To(Succeed())

		proxy := newProxy(
			// dont't give an explicit path but rather define the file in the
			// configLoadingRules precedence chain.
			Kubeconfig{Path: "", Context: context},
			InjectKubeconfigPaths([]string{configFile}),
		)
		conf, err := proxy.GetConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(conf).ToNot(BeNil())
		// asserting on the host of the cluster associated with the
		// context
		g.Expect(conf.Host).To(Equal(expectedHost))
	})
}

func TestProxyCurrentNamespace(t *testing.T) {
	tests := []struct {
		name               string
		kubeconfigPath     string
		kubeconfigContents string
		kubeconfigContext  string
		expectErr          bool
		expectedNamespace  string
	}{
		{
			name:           "return error for invalid kubeconfig path",
			kubeconfigPath: "do-not-exist",
			expectErr:      true,
		},
		{
			name:               "return error for bad kubeconfig contents",
			kubeconfigContents: "management",
			expectErr:          true,
		},
		{
			name:               "return default namespace if unspecified",
			kubeconfigContents: kubeconfig("workload", ""),
			expectErr:          false,
			expectedNamespace:  "default",
		},
		{
			name:               "return error when current-context is empty",
			kubeconfigContents: kubeconfig("", ""),
			expectErr:          true,
		},
		{
			name:               "return error when current-context is incorrect or does not exist",
			kubeconfigContents: kubeconfig("does-not-exist", ""),
			expectErr:          true,
		},
		{
			name:               "return specified namespace for the current context",
			kubeconfigContents: kubeconfig("workload", "mykindns"),
			expectErr:          false,
			expectedNamespace:  "mykindns",
		},
		{
			name:               "return the namespace of the specified context which is different from current context",
			kubeconfigContents: kubeconfig("management", "workload-ns"),
			expectErr:          false,
			kubeconfigContext:  "workload",
			expectedNamespace:  "workload-ns",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var configFile string
			if len(tt.kubeconfigPath) != 0 {
				configFile = tt.kubeconfigPath
			} else {
				dir, err := os.MkdirTemp("", "clusterctl")
				g.Expect(err).NotTo(HaveOccurred())
				defer os.RemoveAll(dir)
				configFile = filepath.Join(dir, ".test-kubeconfig.yaml")
				g.Expect(os.WriteFile(configFile, []byte(tt.kubeconfigContents), 0600)).To(Succeed())
			}

			proxy := newProxy(Kubeconfig{Path: configFile, Context: tt.kubeconfigContext})
			ns, err := proxy.CurrentNamespace()
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(ns).To(Equal(tt.expectedNamespace))
		})
	}
}

func kubeconfig(currentContext, namespace string) string {
	return fmt.Sprintf(`---
apiVersion: v1
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: https://management-server:1234
  name: management
- cluster:
    insecure-skip-tls-verify: true
    server: https://kind-server:38790
  name: workload
contexts:
- context:
    cluster: management
    user: management
    namespace: management-ns
  name: management
- context:
    cluster: workload
    user: workload
    namespace: %s
  name: workload
current-context: %s
kind: Config
preferences: {}
users:
- name: management
  user:
    client-certificate-data: c3R1ZmYK
    client-key-data: c3R1ZmYK
- name: workload
  user:
    client-certificate-data: c3R1ZmYK
    client-key-data: c3R1ZmYK
`, namespace, currentContext)
}
