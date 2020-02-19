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

package internal

import (
	"testing"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

func TestNot(t *testing.T) {
	t.Run("returns false given a machine filter that returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		f := func() MachineFilter {
			return func(machine *clusterv1.Machine) bool {
				return true
			}
		}
		g.Expect(Not(f())(m)).To(gomega.BeFalse())
	})
	t.Run("returns true given a machine filter that returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		f := func() MachineFilter {
			return func(machine *clusterv1.Machine) bool {
				return false
			}
		}
		g.Expect(Not(f())(m)).To(gomega.BeTrue())
	})
}

func TestHasDeletionTimestamp(t *testing.T) {
	t.Run("machine with deletion timestamp returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		now := metav1.Now()
		m.SetDeletionTimestamp(&now)
		g.Expect(HasDeletionTimestamp(m)).To(gomega.BeTrue())
	})
	t.Run("machine with nil deletion timestamp returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(HasDeletionTimestamp(m)).To(gomega.BeFalse())
	})
	t.Run("machine with zero deletion timestamp returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		zero := metav1.NewTime(time.Time{})
		m.SetDeletionTimestamp(&zero)
		g.Expect(HasDeletionTimestamp(m)).To(gomega.BeFalse())
	})
}

func TestMatchesConfigurationHash(t *testing.T) {
	t.Run("machine with configuration hash returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetLabels(ControlPlaneLabelsForClusterWithHash("test", "hashValue"))
		g.Expect(MatchesConfigurationHash("hashValue")(m)).To(gomega.BeTrue())
	})
	t.Run("machine with wrong configuration hash returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetLabels(ControlPlaneLabelsForClusterWithHash("test", "notHashValue"))
		g.Expect(MatchesConfigurationHash("hashValue")(m)).To(gomega.BeFalse())
	})
	t.Run("machine without configuration hash returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(MatchesConfigurationHash("hashValue")(m)).To(gomega.BeFalse())
	})
}

func TestOlderThan(t *testing.T) {
	t.Run("machine with creation timestamp older than given returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-1 * time.Hour)))
		now := metav1.Now()
		g.Expect(OlderThan(&now)(m)).To(gomega.BeTrue())
	})
	t.Run("machine with creation timestamp equal to given returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		now := metav1.Now()
		m.SetCreationTimestamp(now)
		g.Expect(OlderThan(&now)(m)).To(gomega.BeFalse())
	})
	t.Run("machine with creation timestamp after given returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(+1 * time.Hour)))
		now := metav1.Now()
		g.Expect(OlderThan(&now)(m)).To(gomega.BeFalse())
	})
}

func TestSelectedForUpgrade(t *testing.T) {
	t.Run("machine with selected for upgrade label returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{controlplanev1.SelectedForUpgradeAnnotation: ""})
		g.Expect(SelectedForUpgrade(m)).To(gomega.BeTrue())
	})
	t.Run("machine with selected for upgrade label with non-empty value returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{controlplanev1.SelectedForUpgradeAnnotation: "blue"})
		g.Expect(SelectedForUpgrade(m)).To(gomega.BeTrue())
	})
	t.Run("machine without selected for upgrade label returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(SelectedForUpgrade(m)).To(gomega.BeFalse())
	})
}

func TestInFailureDomain(t *testing.T) {
	t.Run("machine with given failure domain returns true", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(InFailureDomains(pointer.StringPtr("test"))(m)).To(gomega.BeTrue())
	})
	t.Run("machine with a different failure domain returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("notTest")}}
		g.Expect(InFailureDomains(pointer.StringPtr("test"), pointer.StringPtr("foo"))(m)).To(gomega.BeFalse())
	})
	t.Run("machine without failure domain returns false", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(InFailureDomains(pointer.StringPtr("test"))(m)).To(gomega.BeFalse())
	})
	t.Run("machine without failure domain returns true, when nil used for failure domain", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(InFailureDomains(nil)(m)).To(gomega.BeTrue())
	})
	t.Run("machine with failure domain returns true, when one of multiple failure domains match", func(t *testing.T) {
		g := gomega.NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(InFailureDomains(pointer.StringPtr("foo"), pointer.StringPtr("test"))(m)).To(gomega.BeTrue())
	})
}
