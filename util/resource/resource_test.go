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

package resource

import (
	"math/rand"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	kindsWithPriority = []string{
		"Namespace", "CustomResourceDefinition", "StorageClass", "PersistentVolume",
		"PersistentVolumeClaim", "Secret", "ConfigMap", "ServiceAccount", "LimitRange",
		"Pod", "ReplicaSet", "Endpoint",
	}
	kindsOther = []string{
		"Deployment", "Service", "DaemonSet", "StatefulSet", "Job", "CronJob", "Ingress",
		"NetworkPolicy", "Role", "RoleBinding",
	}
)

func TestSortForCreate(t *testing.T) {
	g := NewWithT(t)

	cm := unstructured.Unstructured{}
	cm.SetKind("ConfigMap")

	ns := unstructured.Unstructured{}
	ns.SetKind("Namespace")

	ep := unstructured.Unstructured{}
	ep.SetKind("Endpoint")

	dp := unstructured.Unstructured{}
	dp.SetKind("Deployment")

	ds := unstructured.Unstructured{}
	ds.SetKind("DaemonSet")

	resources := []unstructured.Unstructured{ds, dp, ep, cm, ns}
	sorted := SortForCreate(resources)
	g.Expect(sorted).To(HaveLen(5))
	g.Expect(sorted[0].GetKind()).To(BeIdenticalTo("Namespace"))
	g.Expect(sorted[1].GetKind()).To(BeIdenticalTo("ConfigMap"))
	g.Expect(sorted[2].GetKind()).To(BeIdenticalTo("Endpoint"))
	// we have no way to determine the order of the last two elements
	g.Expect(sorted[3].GetKind()).To(BeElementOf([]string{"DaemonSet", "Deployment"}))
	g.Expect(sorted[4].GetKind()).To(BeElementOf([]string{"DaemonSet", "Deployment"}))
}

func TestSortForCreateAllShuffle(t *testing.T) {
	g := NewWithT(t)

	resources := make([]unstructured.Unstructured, 0, len(kindsWithPriority)+len(kindsOther))
	for _, kind := range append(kindsWithPriority, kindsOther...) {
		resource := unstructured.Unstructured{}
		resource.SetKind(kind)
		resources = append(resources, resource)
	}
	for j := range 100 {
		// determinically shuffle resources
		rnd := rand.New(rand.NewSource(int64(j))) //nolint:gosec
		rnd.Shuffle(len(resources), func(i, j int) {
			resources[i], resources[j] = resources[j], resources[i]
		})

		sorted := SortForCreate(resources)
		g.Expect(sorted).To(HaveLen(len(kindsWithPriority) + len(kindsOther)))
		// first check that the first len(kindsWithPriority) elements are from
		// the list of kinds with priority (and in order)
		for i, res := range sorted[:len(kindsWithPriority)] {
			g.Expect(res.GetKind()).To(BeIdenticalTo(kindsWithPriority[i]))
		}
		// while the rest of resources can be any of the other kinds
		for _, res := range sorted[len(kindsWithPriority):] {
			g.Expect(kindsOther).To(ContainElement(res.GetKind()))
		}
	}
}
