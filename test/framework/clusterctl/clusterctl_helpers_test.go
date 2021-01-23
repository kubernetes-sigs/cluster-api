/*
Copyright 2021 The Kubernetes Authors.

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

package clusterctl_test

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

func TestApplyAndWaitHook_Do(t *testing.T) {
	cases := []struct {
		Name string
		Test func(g *WithT)
	}{
		{
			Name: "will not panic with nil hook",
			Test: func(g *WithT) {
				var hook clusterctl.ApplyAndWaitHook = nil
				g.Expect(hook.Do(new(clusterctl.ApplyClusterTemplateAndWaitResult))).To(Succeed())
			},
		},
		{
			Name: "if hook is not nil, will pass result",
			Test: func(g *WithT) {
				var actual *clusterctl.ApplyClusterTemplateAndWaitResult
				var hook = clusterctl.ApplyAndWaitHook(func(res *clusterctl.ApplyClusterTemplateAndWaitResult) error {
					actual = res
					return nil
				})

				expected := &clusterctl.ApplyClusterTemplateAndWaitResult{
					Cluster: &clusterv1.Cluster{},
				}
				g.Expect(hook.Do(expected)).To(Succeed())
				g.Expect(expected).To(Equal(actual))
			},
		},
		{
			Name: "non-nil hook will return error",
			Test: func(g *WithT) {
				var hook = clusterctl.ApplyAndWaitHook(func(res *clusterctl.ApplyClusterTemplateAndWaitResult) error {
					return errors.New("boom")
				})

				g.Expect(hook.Do(nil)).To(HaveOccurred())
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			c.Test(NewWithT(t))
		})
	}
}
