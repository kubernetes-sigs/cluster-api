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
	"testing/synctest"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestLimiterInfoAtMostEvery(t *testing.T) {
	const interval = time.Minute

	machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m-1"}}
	key := NewKey("Waiting", WithObjects(machine))

	// Note: The Limiter has to be created outside synctest.Test, otherwise the test would fail because of the cleanup go routine in the cache.
	// Inside synctest.Test time is faked, so time.Sleep advances the clock used by the Limiter without waiting.
	// The cleanup go routine uses the real clock and would drop entries created with the faked clock (~ year 2000),
	// so the Limiters used with synctest.Test have a high TTL.
	const ttlWithFakeTime = 250 * 365 * 24 * time.Hour

	t.Run("logs the first call and then at most once per interval", func(t *testing.T) {
		limiter := newWithTTL(t.Context(), ttlWithFakeTime)
		synctest.Test(t, func(t *testing.T) {
			g := NewWithT(t)

			log, lines := newTestLogger(0)

			limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
			g.Expect(*lines).To(HaveLen(1))
			g.Expect((*lines)[0]).ToNot(ContainSubstring("duration"))

			// Within the interval nothing is logged at the default verbosity.
			time.Sleep(30 * time.Second)
			limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
			g.Expect(*lines).To(HaveLen(1))

			// After the interval the line is logged again, with the time since the first call.
			time.Sleep(31 * time.Second)
			limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
			g.Expect(*lines).To(HaveLen(2))
			g.Expect((*lines)[1]).To(ContainSubstring(`"duration"="1m1s"`))

			// The interval restarts from the last logged line, the duration still counts from the first call.
			time.Sleep(30 * time.Second)
			limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
			g.Expect(*lines).To(HaveLen(2))
			time.Sleep(31 * time.Second)
			limiter.InfoAtMostEvery(log, interval, key, "Waiting for something", "Machine", "ns/m-1")
			g.Expect(*lines).To(HaveLen(3))
			g.Expect((*lines)[2]).To(ContainSubstring(`"duration"="2m2s"`))
		})
	})

	t.Run("logs the calls skipped by the limiter at a high verbosity", func(t *testing.T) {
		g := NewWithT(t)
		limiter := New(t.Context())

		logV4, linesV4 := newTestLogger(4)
		for range 3 {
			limiter.InfoAtMostEvery(logV4, interval, key, "Waiting for something")
		}
		g.Expect(*linesV4).To(HaveLen(1))

		logV5, linesV5 := newTestLogger(5)
		// Use a different key so the first call is not skipped, like above.
		key5 := NewKey("Waiting5", WithObjects(machine))
		for range 3 {
			limiter.InfoAtMostEvery(logV5, interval, key5, "Waiting for something")
		}
		g.Expect(*linesV5).To(HaveLen(3))
	})

	t.Run("rate limits each key independently", func(t *testing.T) {
		g := NewWithT(t)

		limiter := New(t.Context())
		log, lines := newTestLogger(0)

		otherMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m-2"}}
		limiter.InfoAtMostEvery(log, interval, key, "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(otherMachine)), "Waiting for something")
		limiter.InfoAtMostEvery(log, interval, NewKey("WaitingForOther", WithObjects(machine)), "Waiting for something else")
		g.Expect(*lines).To(HaveLen(3))
	})

	t.Run("logs immediately when the values change", func(t *testing.T) {
		limiter := newWithTTL(t.Context(), ttlWithFakeTime)
		synctest.Test(t, func(t *testing.T) {
			g := NewWithT(t)

			log, lines := newTestLogger(0)

			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			g.Expect(*lines).To(HaveLen(1))

			// A change of the values is logged immediately, and the duration counts from the first call because we are still waiting.
			time.Sleep(10 * time.Second)
			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("b")), "Waiting for something")
			g.Expect(*lines).To(HaveLen(2))
			g.Expect((*lines)[1]).To(ContainSubstring(`"duration"="10s"`))

			// Going back to the previous values is a change as well.
			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			g.Expect(*lines).To(HaveLen(3))
		})
	})

	t.Run("logs again at Info after Forget", func(t *testing.T) {
		limiter := newWithTTL(t.Context(), ttlWithFakeTime)
		synctest.Test(t, func(t *testing.T) {
			g := NewWithT(t)

			log, lines := newTestLogger(0)

			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			time.Sleep(10 * time.Second)
			// Note: Forget does not need the values the line was logged with.
			limiter.Forget(key)
			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			g.Expect(*lines).To(HaveLen(2))

			// The duration restarts after Forget.
			time.Sleep(interval)
			limiter.InfoAtMostEvery(log, interval, NewKey("Waiting", WithObjects(machine), WithValues("a")), "Waiting for something")
			g.Expect(*lines).To(HaveLen(3))
			g.Expect((*lines)[2]).To(ContainSubstring(`"duration"="1m0s"`))
		})
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

func newTestLogger(verbosity int) (logr.Logger, *[]string) {
	lines := &[]string{}
	return funcr.New(func(_, args string) {
		*lines = append(*lines, args)
	}, funcr.Options{Verbosity: verbosity}), lines
}
