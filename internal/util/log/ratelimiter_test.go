/*
Copyright The Kubernetes Authors.

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

package log

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clocktesting "k8s.io/utils/clock/testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func newTestLogger(verbosity int) (logr.Logger, *[]string) {
	lines := &[]string{}
	return funcr.New(func(_, args string) {
		*lines = append(*lines, args)
	}, funcr.Options{Verbosity: verbosity}), lines
}

func TestLimiterInfoAtMostEvery(t *testing.T) {
	const interval = time.Minute

	machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m-1"}}
	key := NewKey("Waiting", WithObjects(machine))

	t.Run("logs the first call and then at most once per interval", func(t *testing.T) {
		g := NewWithT(t)

		fakeClock := clocktesting.NewFakeClock(time.Now())
		limiter := New(t.Context(), 10*time.Minute, WithClock(fakeClock))
		log, lines := newTestLogger(0)

		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
		g.Expect(*lines).To(HaveLen(1))
		g.Expect((*lines)[0]).ToNot(ContainSubstring("duration"))

		// Within the interval nothing is logged at the default verbosity.
		fakeClock.Step(30 * time.Second)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
		g.Expect(*lines).To(HaveLen(1))

		// After the interval the line is logged again, with the time since the first call.
		fakeClock.Step(31 * time.Second)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
		g.Expect(*lines).To(HaveLen(2))
		g.Expect((*lines)[1]).To(ContainSubstring(`"duration"="1m1s"`))

		// The interval restarts from the last logged line, the duration still counts from the first call.
		fakeClock.Step(30 * time.Second)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
		g.Expect(*lines).To(HaveLen(2))
		fakeClock.Step(31 * time.Second)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
		g.Expect(*lines).To(HaveLen(3))
		g.Expect((*lines)[2]).To(ContainSubstring(`"duration"="2m2s"`))
	})

	t.Run("logs the calls skipped by the limiter at a high verbosity", func(t *testing.T) {
		g := NewWithT(t)

		limiter := New(t.Context(), 10*time.Minute, WithClock(clocktesting.NewFakeClock(time.Now())))
		log, lines := newTestLogger(repeatedLogVerbosity)

		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		g.Expect(*lines).To(HaveLen(3))
	})

	t.Run("rate limits each key independently", func(t *testing.T) {
		g := NewWithT(t)

		limiter := New(t.Context(), 10*time.Minute, WithClock(clocktesting.NewFakeClock(time.Now())))
		log, lines := newTestLogger(0)

		otherMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m-2"}}
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(otherMachine)), "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, NewKey("WaitingForOther", WithObjects(machine)), "Waiting for something else")
		g.Expect(*lines).To(HaveLen(3))
	})

	t.Run("logs immediately when the values change", func(t *testing.T) {
		g := NewWithT(t)

		limiter := New(t.Context(), 10*time.Minute, WithClock(clocktesting.NewFakeClock(time.Now())))
		log, lines := newTestLogger(0)

		limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
		g.Expect(*lines).To(HaveLen(1))
		limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("b")), "Waiting for something")
		g.Expect(*lines).To(HaveLen(2))
	})

	t.Run("logs again at Info after Forget", func(t *testing.T) {
		g := NewWithT(t)

		fakeClock := clocktesting.NewFakeClock(time.Now())
		limiter := New(t.Context(), 10*time.Minute, WithClock(fakeClock))
		log, lines := newTestLogger(0)

		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		fakeClock.Step(10 * time.Second)
		limiter.Forget(key)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		g.Expect(*lines).To(HaveLen(2))

		// The duration restarts after Forget.
		fakeClock.Step(interval)
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		g.Expect(*lines).To(HaveLen(3))
		g.Expect((*lines)[2]).To(ContainSubstring(`"duration"="1m0s"`))
	})

	t.Run("logs every call if the limiter is nil", func(t *testing.T) {
		g := NewWithT(t)

		var limiter *Limiter
		log, lines := newTestLogger(0)

		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.Forget(key)
		g.Expect(*lines).To(HaveLen(2))
	})
}
