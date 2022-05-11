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

package v1alpha1

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha2"
)

func TestConversion(t *testing.T) {
	g := NewWithT(t)

	var c = runtimecatalog.New()
	_ = AddToCatalog(c)
	_ = v1alpha2.AddToCatalog(c)

	t.Run("down-convert FakeRequest v1alpha2 to v1alpha1", func(t *testing.T) {
		request := &v1alpha2.FakeRequest{Cluster: clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		}}}
		requestLocal := &FakeRequest{}

		g.Expect(c.Convert(request, requestLocal, context.Background())).To(Succeed())
		g.Expect(requestLocal.Cluster.GetName()).To(Equal(request.Cluster.Name))
	})

	t.Run("up-convert FakeResponse v1alpha1 to v1alpha2", func(t *testing.T) {
		responseLocal := &FakeResponse{
			First:  1,
			Second: "foo",
		}
		response := &v1alpha2.FakeResponse{}
		g.Expect(c.Convert(responseLocal, response, context.Background())).To(Succeed())

		g.Expect(response.First).To(Equal(responseLocal.First))
		g.Expect(response.Second).To(Equal(responseLocal.Second))
	})
}
