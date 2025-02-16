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
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
)

var scheme = runtime.NewScheme()

func init() {
	_ = cloudv1.AddToScheme(scheme)
}

func Test_cache_scale(t *testing.T) {
	t.Skip()
	g := NewWithT(t)

	ctrl.SetLogger(klog.Background())

	resourceGroups := 1000
	objectsForResourceGroups := 500
	operationFrequencyForResourceGroup := 10 * time.Millisecond
	testDuration := 2 * time.Minute

	var (
		createCount atomic.Uint64
		getCount    atomic.Uint64
		listCount   atomic.Uint64
		deleteCount atomic.Uint64
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	c := NewCache(scheme).(*cache)
	c.syncPeriod = testDuration / 10                        // force a shorter sync period
	c.garbageCollectorRequeueAfter = 500 * time.Millisecond // force a shorter gc requeueAfter
	err := c.Start(ctx)
	g.Expect(err).ToNot(HaveOccurred())

	g.Eventually(func() bool {
		return c.started
	}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(), "manager should start")

	machineName := func(j int) string {
		return fmt.Sprintf("machine-%d", j)
	}

	for i := range resourceGroups {
		resourceGroup := fmt.Sprintf("resourceGroup-%d", i)
		c.AddResourceGroup(resourceGroup)

		go func() {
			for {
				select {
				case <-time.After(wait.Jitter(operationFrequencyForResourceGroup, 1)):
					operation := rand.Intn(3)                   //nolint:gosec // Intentionally using a weak random number generator here.
					item := rand.Intn(objectsForResourceGroups) //nolint:gosec // Intentionally using a weak random number generator here.
					switch operation {
					case 0: // create or get
						machine := &cloudv1.CloudMachine{
							ObjectMeta: metav1.ObjectMeta{
								Name: machineName(item),
							},
						}
						err := c.Create(resourceGroup, machine)
						if apierrors.IsAlreadyExists(err) {
							if err = c.Get(resourceGroup, types.NamespacedName{Name: machineName(item)}, machine); err == nil {
								getCount.Add(1)
								continue
							}
						}
						g.Expect(err).ToNot(HaveOccurred())
						createCount.Add(1)
					case 1: // list
						obj := &cloudv1.CloudMachineList{}
						err := c.List(resourceGroup, obj)
						g.Expect(err).ToNot(HaveOccurred())
						listCount.Add(1)
					case 2: // delete
						g.Expect(err).ToNot(HaveOccurred())
						machine := &cloudv1.CloudMachine{
							ObjectMeta: metav1.ObjectMeta{
								Name: machineName(item),
							},
						}
						err := c.Delete(resourceGroup, machine)
						if apierrors.IsNotFound(err) {
							continue
						}
						g.Expect(err).ToNot(HaveOccurred())
						deleteCount.Add(1)
					}

				case <-ctx.Done():
					return
				}
			}
		}()
	}

	time.Sleep(testDuration)

	t.Log("createCount", createCount.Load(), "getCount", getCount.Load(), "listCount", listCount.Load(), "deleteCount", deleteCount.Load())

	cancel()
}
