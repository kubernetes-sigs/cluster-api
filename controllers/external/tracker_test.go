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

package external

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	logger = log.NullLogger{}
)

type fakeController struct {
	controller.Controller
}

type watchCountController struct {
	// can not directly embed an interface when a pointer receiver is
	// used in any of the overriding methods.
	*fakeController
	// no.of times Watch was called
	count      int
	raiseError bool
}

func newWatchCountController(raiseError bool) *watchCountController {
	return &watchCountController{
		raiseError: raiseError,
	}
}

func (c *watchCountController) Watch(_ source.Source, _ handler.EventHandler, _ ...predicate.Predicate) error {
	c.count++
	if c.raiseError {
		return errors.New("injected failure")
	}
	return nil
}

func TestRetryWatch(t *testing.T) {
	g := NewWithT(t)
	ctrl := newWatchCountController(true)
	tracker := ObjectTracker{Controller: ctrl}

	err := tracker.Watch(logger, &clusterv1.Cluster{}, nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(ctrl.count).Should(Equal(1))
	// Calling Watch on same Object kind that failed earlier should be retryable.
	err = tracker.Watch(logger, &clusterv1.Cluster{}, nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(ctrl.count).Should(Equal(2))
}

func TestWatchMultipleTimes(t *testing.T) {
	g := NewWithT(t)
	ctrl := &watchCountController{}
	tracker := ObjectTracker{Controller: ctrl}

	obj := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.Version,
		},
	}
	err := tracker.Watch(logger, obj, nil)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ctrl.count).Should(Equal(1))
	// Calling Watch on same Object kind should not register watch again.
	err = tracker.Watch(logger, obj, nil)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ctrl.count).Should(Equal(1))
}
