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

// Package log provides utils to reduce the noise of log lines that are repeated on every reconcile while
// a controller is waiting for something.
package log

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/util/cache"
)

// repeatedLogVerbosity is the verbosity at which log lines skipped by the Limiter are still logged,
// so that all the occurrences are available when debugging.
const repeatedLogVerbosity = 5

// Key identifies a log line that is rate limited, e.g. "waiting for the infrastructure of Machine ns/m-1".
// Use NewKey to build it.
type Key string

type keyParts struct {
	objects []string
	values  []string
}

// KeyOption customizes a Key.
type KeyOption func(*keyParts)

// WithObjects adds the given objects to a Key, so that the same log line about different objects is rate limited independently.
func WithObjects(objs ...client.Object) KeyOption {
	return func(p *keyParts) {
		for _, obj := range objs {
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			if kind == "" {
				// Note: Typed objects usually don't have the Kind set.
				// Using "%T" is considered an acceptable workaround for now, and it also works for provider's kind not registered in the scheme.
				kind = fmt.Sprintf("%T", obj)
			}
			p.objects = append(p.objects, fmt.Sprintf("%s %s", kind, klog.KObj(obj)))
		}
	}
}

// WithValues adds the given values to a Key, so that a change of the values, e.g. a different reason why we are
// waiting, is logged immediately instead of waiting for the interval to elapse.
func WithValues(values ...string) KeyOption {
	return func(p *keyParts) {
		p.values = append(p.values, values...)
	}
}

// NewKey returns a Key for the given message identifier.
func NewKey(messageIdentifier string, opts ...KeyOption) Key {
	parts := &keyParts{}
	for _, opt := range opts {
		opt(parts)
	}

	key := messageIdentifier
	for _, obj := range parts.objects {
		key += "|" + obj
	}
	// Note: Values are quoted, so values containing the separator cannot collide with a different list of values.
	for _, value := range parts.values {
		key += "|" + strconv.Quote(value)
	}
	return Key(key)
}

type entry struct {
	key        Key
	firstSeen  time.Time
	lastLogged time.Time
}

// Key returns the cache key of an entry.
func (e entry) Key() string {
	return string(e.key)
}

var _ cache.Entry = entry{}

// Limiter logs a line at most once per interval.
// A nil Limiter does not rate limit, so it is safe to use in unit tests that don't set it up.
type Limiter struct {
	cache cache.Cache[entry]
	clock clock.PassiveClock
}

// Option customizes a Limiter.
type Option func(*Limiter)

// WithClock sets the clock used by a Limiter; it is used to control the time in unit tests.
func WithClock(c clock.PassiveClock) Option {
	return func(l *Limiter) {
		l.clock = c
	}
}

// New creates a Limiter.
// ttl is the duration after which the state of a log line is dropped if the line is not logged anymore.
func New(ctx context.Context, ttl time.Duration, opts ...Option) *Limiter {
	l := &Limiter{
		cache: cache.New[entry](ctx, ttl),
		clock: clock.RealClock{},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// InfoAtMostEvery logs the Info message the first time it is called for a given key, and then at most once per interval.
// The calls skipped because the interval did not elapse are logged at a higher verbosity, so all the occurrences are
// still available when debugging.
func (l *Limiter) InfoAtMostEvery(log logr.Logger, interval time.Duration, key Key, msg string, keysAndValues ...any) {
	if l == nil {
		log.Info(msg, keysAndValues...)
		return
	}

	now := l.clock.Now()
	e, found := l.cache.Has(string(key))
	if !found {
		l.cache.Add(entry{key: key, firstSeen: now, lastLogged: now})
		log.Info(msg, keysAndValues...)
		return
	}

	if now.Sub(e.lastLogged) < interval {
		log.V(repeatedLogVerbosity).Info(msg, keysAndValues...)
		return
	}

	// Note: Adding the entry again also extends its expiration.
	e.lastLogged = now
	l.cache.Add(e)
	log.Info(msg, append(slices.Clone(keysAndValues), "duration", now.Sub(e.firstSeen).Truncate(time.Second))...)
}

// Forget drops the state of a log key.
// It should be called when the wait ends.
func (l *Limiter) Forget(key Key) {
	if l == nil {
		return
	}
	l.cache.Delete(entry{key: key})
}
