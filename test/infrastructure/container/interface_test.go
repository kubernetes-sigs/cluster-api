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

package container

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func TestFilterBuildKeyValue(t *testing.T) {
	g := NewWithT(t)

	filters := FilterBuilder{}
	filters.AddKeyValue("key1", "value1")

	g.Expect(filters).To(Equal(FilterBuilder{"key1": {"value1": []string{""}}}))
}

func TestFilterBuildKeyNameValue(t *testing.T) {
	g := NewWithT(t)

	filters := FilterBuilder{}
	filters.AddKeyNameValue("key1", "name1", "value1")

	g.Expect(filters).To(Equal(FilterBuilder{"key1": {"name1": []string{"value1"}}}))
}

func TestFakeContext(t *testing.T) {
	g := NewWithT(t)
	fake := FakeRuntime{}
	ctx := RuntimeInto(context.Background(), &fake)
	rtc, err := RuntimeFrom(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	_, ok := rtc.(*FakeRuntime)
	g.Expect(ok).To(BeTrue())
}

func TestDockerContext(t *testing.T) {
	g := NewWithT(t)
	docker := dockerRuntime{}
	ctx := RuntimeInto(context.Background(), &docker)
	rtc, err := RuntimeFrom(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	_, ok := rtc.(*dockerRuntime)
	g.Expect(ok).To(BeTrue())
}

func TestInvalidContext(t *testing.T) {
	g := NewWithT(t)
	_, err := RuntimeFrom(context.Background())
	g.Expect(err).Should(HaveOccurred())
}
