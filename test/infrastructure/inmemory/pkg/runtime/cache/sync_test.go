/*
Copyright 2023 The Kubernetes Authors.

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

package cache

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
)

func Test_cache_sync(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	c := NewCache(scheme).(*cache)
	c.syncPeriod = 5 * time.Second // force a shorter sync period
	h := &fakeHandler{}
	i, err := c.GetInformer(ctx, &cloudv1.CloudMachine{})
	g.Expect(err).ToNot(HaveOccurred())
	err = i.AddEventHandler(h)
	g.Expect(err).ToNot(HaveOccurred())

	err = c.Start(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.started).To(BeTrue())

	c.AddResourceGroup("foo")

	obj := &cloudv1.CloudMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "baz",
		},
	}
	err = c.Create("foo", obj)
	g.Expect(err).ToNot(HaveOccurred())

	objBefore := &cloudv1.CloudMachine{}
	err = c.Get("foo", types.NamespacedName{Name: "baz"}, objBefore)
	g.Expect(err).ToNot(HaveOccurred())

	lastSyncBefore, ok := lastSyncTimeAnnotationValue(objBefore)
	g.Expect(ok).To(BeTrue())

	g.Eventually(func() bool {
		objAfter := &cloudv1.CloudMachine{}
		err = c.Get("foo", types.NamespacedName{Name: "baz"}, objAfter)
		if err != nil {
			return false
		}
		lastSyncAfter, ok := lastSyncTimeAnnotationValue(objAfter)
		if !ok {
			return false
		}
		if lastSyncBefore != lastSyncAfter {
			return true
		}
		return false
	}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(), "object should be synced")

	g.Expect(h.Events()).To(ContainElement("foo, CloudMachine=baz, Updated"))

	cancel()
}
