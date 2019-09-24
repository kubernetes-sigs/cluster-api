/*
Copyright 2019 The Kubernetes Authors.

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
package phases

import (
	"fmt"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
)

const (
	InfrastructureAPIVersion    = "infrastructure.cluster.x-k8s.io/v1alpha2"
	KindProviderMachineTemplate = "ProviderMachineTemplate"
	KindProviderMachine         = "ProviderMachine"
	KindProviderCluster         = "ProviderCluster"
)

// sourcer map keys are namespaces
// This is intentionally a different implementation than clusterclient
type sourcer struct {
	clusters         map[string][]*clusterv1.Cluster
	clusterResources map[string][]*unstructured.Unstructured

	machines         map[string][]*clusterv1.Machine
	machineResources map[string][]*unstructured.Unstructured

	machineDeployments       map[string][]*clusterv1.MachineDeployment
	machineSets              map[string][]*clusterv1.MachineSet
	machineTemplateResources map[string][]*unstructured.Unstructured

	secrets map[string][]*corev1.Secret
}

func newSourcer() *sourcer {
	return &sourcer{
		clusters:                 make(map[string][]*clusterv1.Cluster),
		clusterResources:         make(map[string][]*unstructured.Unstructured),
		machines:                 make(map[string][]*clusterv1.Machine),
		machineResources:         make(map[string][]*unstructured.Unstructured),
		machineDeployments:       make(map[string][]*clusterv1.MachineDeployment),
		machineSets:              make(map[string][]*clusterv1.MachineSet),
		machineTemplateResources: make(map[string][]*unstructured.Unstructured),
		secrets:                  make(map[string][]*corev1.Secret),
	}
}

func (s *sourcer) WithCluster(ns, name string) *sourcer {
	s.clusters[ns] = append(s.clusters[ns], &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: InfrastructureAPIVersion,
				Kind:       KindProviderCluster,
				Name:       name,
				Namespace:  ns,
			},
		},
	})

	s.clusterResources[ns] = append(s.clusterResources[ns], &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderCluster,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})

	s.secrets[ns] = append(s.secrets[ns], &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-kubeconfig",
			Namespace: ns,
		},
	})

	return s
}

func (s *sourcer) WithMachineDeployment(ns, cluster, name string) *sourcer {
	md := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: InfrastructureAPIVersion,
						Kind:       KindProviderMachineTemplate,
						Name:       name,
						Namespace:  ns,
					},
				},
			},
		},
	}

	if cluster != "" {
		md.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster}
		blockOwnerDeletion := true
		md.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}

	s.machineTemplateResources[ns] = append(s.machineTemplateResources[ns], &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderMachineTemplate,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})
	s.machineDeployments[ns] = append(s.machineDeployments[ns], &md)
	return s
}

func (s *sourcer) WithMachineSet(ns, cluster, md, name string) *sourcer {
	ms := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	if cluster != "" {
		ms.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster}
		blockOwnerDeletion := true
		ms.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}

	if md != "" {
		isController := true
		ms.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.Version,
				Kind:       "MachineDeployment",
				Name:       md,
				Controller: &isController,
			},
		}
	} else {
		ms.Spec.Template.Spec.InfrastructureRef = corev1.ObjectReference{
			APIVersion: InfrastructureAPIVersion,
			Kind:       KindProviderMachineTemplate,
			Name:       name,
			Namespace:  ns,
		}
		s.machineTemplateResources[ns] = append(s.machineTemplateResources[ns], &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": InfrastructureAPIVersion,
				"kind":       KindProviderMachineTemplate,
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": ns,
				},
			},
		})
	}
	s.machineSets[ns] = append(s.machineSets[ns], &ms)
	return s
}

func (s *sourcer) WithMachine(ns, cluster, ms, name string) *sourcer {
	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: InfrastructureAPIVersion,
				Kind:       KindProviderMachine,
				Name:       name,
				Namespace:  ns,
			},
		},
	}

	if cluster != "" {
		m.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster}
		blockOwnerDeletion := true
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}
	if ms != "" {
		isController := true
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.Version,
				Kind:       "MachineSet",
				Name:       ms,
				Controller: &isController,
			},
		}
	}

	s.machineResources[ns] = append(s.machineResources[ns], &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderMachine,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})
	s.machines[ns] = append(s.machines[ns], &m)
	return s
}

// Interface implementation below

func (s *sourcer) Delete(string) error {
	return nil
}

func (s *sourcer) ForceDeleteCluster(ns, name string) error {
	newClusters := []*clusterv1.Cluster{}
	for _, d := range s.clusters[ns] {
		if d.Name != name {
			newClusters = append(newClusters, d)
		}
	}
	s.clusters[ns] = newClusters
	return nil
}

func (s *sourcer) ForceDeleteMachine(ns, name string) error {
	newMachines := []*clusterv1.Machine{}
	for _, d := range s.machines[ns] {
		if d.Name != name {
			newMachines = append(newMachines, d)
		}
	}
	s.machines[ns] = newMachines
	return nil
}

func (s *sourcer) ForceDeleteMachineDeployment(ns, name string) error {
	newDeployments := []*clusterv1.MachineDeployment{}
	for _, d := range s.machineDeployments[ns] {
		if d.Name != name {
			newDeployments = append(newDeployments, d)
		}
	}
	s.machineDeployments[ns] = newDeployments
	return nil
}

func (s *sourcer) ForceDeleteMachineSet(ns, name string) error {
	newSets := []*clusterv1.MachineSet{}
	for _, d := range s.machineSets[ns] {
		if d.Name != name {
			newSets = append(newSets, d)
		}
	}
	s.machineSets[ns] = newSets
	return nil
}

func (s *sourcer) GetClusters(ns string) ([]*clusterv1.Cluster, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.Cluster{}
		for _, clusters := range s.clusters {
			out = append(out, clusters...)
		}
		return out, nil
	}
	return s.clusters[ns], nil
}

func (s *sourcer) GetMachineDeployments(ns string) ([]*clusterv1.MachineDeployment, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.MachineDeployment{}
		for _, mds := range s.machineDeployments {
			out = append(out, mds...)
		}
		return out, nil
	}
	return s.machineDeployments[ns], nil
}

func (s *sourcer) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	var mds []*clusterv1.MachineDeployment
	for _, md := range s.machineDeployments[cluster.Namespace] {
		if md.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			mds = append(mds, md)
		}
	}
	return mds, nil
}

func (s *sourcer) GetMachines(ns string) ([]*clusterv1.Machine, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.Machine{}
		for _, machines := range s.machines {
			out = append(out, machines...)
		}
		return out, nil
	}
	return s.machines[ns], nil
}

func (s *sourcer) GetMachineSets(ns string) ([]*clusterv1.MachineSet, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.MachineSet{}
		for _, machineSets := range s.machineSets {
			out = append(out, machineSets...)
		}
		return out, nil
	}
	return s.machineSets[ns], nil
}

func (s *sourcer) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	var machineSets []*clusterv1.MachineSet
	for _, ms := range s.machineSets[cluster.Namespace] {
		if ms.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			machineSets = append(machineSets, ms)
		}
	}
	return machineSets, nil
}

func (s *sourcer) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	var machines []*clusterv1.Machine
	for _, m := range s.machines[cluster.Namespace] {
		if m.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			machines = append(machines, m)
		}
	}
	return machines, nil
}

func (s *sourcer) GetMachineSetsForMachineDeployment(d *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	var machineSets []*clusterv1.MachineSet
	for _, ms := range s.machineSets[d.Namespace] {
		for _, or := range ms.OwnerReferences {
			if or.Kind == "MachineDeployment" && or.Name == d.Name {
				machineSets = append(machineSets, ms)
			}
		}
	}
	return machineSets, nil
}

func (s *sourcer) GetUnstructuredObject(u *unstructured.Unstructured) error {
	ns := u.GetNamespace()
	switch u.GetKind() {
	case KindProviderCluster:
		for _, d := range s.clusterResources[ns] {
			if d.GetName() == u.GetName() {
				d.DeepCopyInto(u)
				return nil
			}
		}
	case KindProviderMachine:
		for _, d := range s.machineResources[ns] {
			if d.GetName() == u.GetName() {
				d.DeepCopyInto(u)
				return nil
			}
		}
	case KindProviderMachineTemplate:
		for _, d := range s.machineTemplateResources[ns] {
			if d.GetName() == u.GetName() {
				d.DeepCopyInto(u)
				return nil
			}
		}
	}
	return errors.New("not found")
}

func (s *sourcer) GetMachinesForMachineSet(ms *clusterv1.MachineSet) ([]*clusterv1.Machine, error) {
	var machines []*clusterv1.Machine
	for _, m := range s.machines[ms.Namespace] {
		for _, or := range m.OwnerReferences {
			if or.Kind == "MachineSet" && or.Name == ms.Name {
				machines = append(machines, m)
			}
		}
	}
	return machines, nil
}

func (s *sourcer) ScaleDeployment(string, string, int32) error {
	return nil
}

func (s *sourcer) WaitForClusterV1alpha2Ready() error {
	return nil
}

func (s *sourcer) ForceDeleteUnstructuredObject(u *unstructured.Unstructured) error {
	ns := u.GetNamespace()

	newResources := []*unstructured.Unstructured{}

	switch u.GetKind() {
	case KindProviderCluster:
		for _, d := range s.clusterResources[ns] {
			if d.GetName() != u.GetName() {
				newResources = append(newResources, d)
			}
		}
		s.clusterResources[ns] = newResources
	case KindProviderMachine:
		for _, d := range s.machineResources[ns] {
			if d.GetName() != u.GetName() {
				newResources = append(newResources, d)
			}
		}
		s.machineResources[ns] = newResources
	case KindProviderMachineTemplate:
		for _, d := range s.machineTemplateResources[ns] {
			if d.GetName() != u.GetName() {
				newResources = append(newResources, d)
			}
		}
		s.machineTemplateResources[ns] = newResources
	}

	return nil
}

func (s *sourcer) GetClusterSecrets(c *clusterv1.Cluster) ([]*corev1.Secret, error) {
	res := []*corev1.Secret{}
	for _, d := range s.secrets[c.Namespace] {
		if strings.HasPrefix(d.Name, c.Name) {
			res = append(res, d)
		}
	}
	return res, nil
}

func (s *sourcer) ForceDeleteSecret(ns, name string) error {
	newSecrets := []*corev1.Secret{}
	for _, d := range s.secrets[ns] {
		if d.Name != name {
			newSecrets = append(newSecrets, d)
		}
	}
	s.secrets[ns] = newSecrets
	return nil
}

type target struct {
	clusters         map[string][]*clusterv1.Cluster
	clusterResources map[string][]*unstructured.Unstructured

	machines         map[string][]*clusterv1.Machine
	machineResources map[string][]*unstructured.Unstructured

	machineDeployments       map[string][]*clusterv1.MachineDeployment
	machineSets              map[string][]*clusterv1.MachineSet
	machineTemplateResources map[string][]*unstructured.Unstructured

	secrets map[string][]*corev1.Secret
}

func newTarget() *target {
	return &target{
		clusters:                 make(map[string][]*clusterv1.Cluster),
		clusterResources:         make(map[string][]*unstructured.Unstructured),
		machines:                 make(map[string][]*clusterv1.Machine),
		machineResources:         make(map[string][]*unstructured.Unstructured),
		machineDeployments:       make(map[string][]*clusterv1.MachineDeployment),
		machineSets:              make(map[string][]*clusterv1.MachineSet),
		machineTemplateResources: make(map[string][]*unstructured.Unstructured),
		secrets:                  make(map[string][]*corev1.Secret),
	}
}

func (t *target) Apply(string) error {
	return nil
}

func (t *target) CreateClusterObject(c *clusterv1.Cluster) error {
	t.clusters[c.Namespace] = append(t.clusters[c.Namespace], c)
	return nil
}

func (t *target) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, ns string) error {
	t.machineDeployments[ns] = append(t.machineDeployments[ns], deployments...)
	return nil
}

func (t *target) CreateMachines(machines []*clusterv1.Machine, ns string) error {
	t.machines[ns] = append(t.machines[ns], machines...)
	return nil
}

func (t *target) CreateMachineSets(ms []*clusterv1.MachineSet, ns string) error {
	t.machineSets[ns] = append(t.machineSets[ns], ms...)
	return nil
}

func (t *target) CreateSecret(secret *corev1.Secret) error {
	t.secrets[secret.Namespace] = append(t.secrets[secret.Namespace], secret)
	return nil
}

func (t *target) EnsureNamespace(string) error {
	return nil
}

func (t *target) GetCluster(name, ns string) (*clusterv1.Cluster, error) {
	return nil, nil
}

func (t *target) GetMachineDeployment(ns, name string) (*clusterv1.MachineDeployment, error) {
	for _, deployment := range t.machineDeployments[ns] {
		if deployment.Name == name {
			return deployment, nil
		}
	}
	return nil, fmt.Errorf("no machine deployment found in ns %q with name %q", ns, name)
}

func (t *target) GetMachineSet(ns, name string) (*clusterv1.MachineSet, error) {
	for _, ms := range t.machineSets[ns] {
		if ms.Name == name {
			return ms, nil
		}
	}
	return nil, fmt.Errorf("no machineset found with name %q in namespace %q", ns, name)
}

func (t *target) WaitForClusterV1alpha2Ready() error {
	return nil
}

func (t *target) SetClusterOwnerRef(obj runtime.Object, cluster *clusterv1.Cluster) error {
	return nil
}

func (t *target) CreateUnstructuredObject(u *unstructured.Unstructured) error {
	ns := u.GetNamespace()

	switch u.GetKind() {
	case KindProviderCluster:
		t.clusterResources[ns] = append(t.clusterResources[ns], u)
		return nil
	case KindProviderMachine:
		t.machineResources[ns] = append(t.machineResources[ns], u)
		return nil
	case KindProviderMachineTemplate:
		t.machineTemplateResources[ns] = append(t.machineTemplateResources[ns], u)
		return nil
	}

	return errors.Errorf("unknown object kind %s", u.GetKind())
}

type providerComponents struct {
	// TODO use this then render them as YAML in the String function
	//	sets []*appsv1.StatefulSet
	names []string
}

func (p *providerComponents) String() string {
	yamls := []string{}
	for _, name := range p.names {
		yamls = append(yamls, fmt.Sprintf(`kind: StatefulSet
apiVersion: apps/v1
metadata:
    name: %s
`, name))
	}
	return strings.Join(yamls, "---")
}

func TestPivot(t *testing.T) {
	pc := &providerComponents{
		names: []string{"test1", "test2"},
	}

	ns1 := "ns1"
	ns2 := "ns2"

	source := newSourcer().
		WithCluster(ns1, "cluster1").
		WithMachineDeployment(ns1, "cluster1", "deployment1").
		WithMachineSet(ns1, "cluster1", "deployment1", "machineset1").
		WithMachine(ns1, "cluster1", "machineset1", "machine1").
		WithMachine(ns1, "cluster1", "machineset1", "machine2").
		WithMachineSet(ns1, "cluster1", "machinedeployment1", "machineset2").
		WithMachine(ns1, "cluster1", "machineset2", "machine3").
		WithMachineSet(ns1, "cluster1", "", "machineset3").
		WithMachine(ns1, "cluster1", "machineset3", "machine4").
		WithMachine(ns1, "cluster1", "", "machine5").
		WithMachineDeployment(ns1, "", "deployment2").
		WithMachineSet(ns1, "", "deployment2", "machineset4").
		WithMachine(ns1, "", "machineset4", "machine6").
		WithMachineSet(ns1, "", "", "machineset5").
		WithMachine(ns1, "", "machineset5", "machine7").
		WithMachine(ns1, "", "", "machine8").
		WithMachine(ns2, "", "", "machine9")

	target := newTarget()

	expectedClusters := len(source.clusters[ns1]) + len(source.clusters[ns2])
	expectedMachineDeployments := len(source.machineDeployments[ns1]) + len(source.machineDeployments[ns2])
	expectedMachineSets := len(source.machineSets[ns1]) + len(source.machineSets[ns2])
	expectedProviderMachineTemplates := len(source.machineTemplateResources[ns1]) + len(source.machineTemplateResources[ns2])
	expectedMachines := len(source.machines[ns1]) + len(source.machines[ns2])

	if err := Pivot(source, target, pc.String()); err != nil {
		t.Fatalf("did not expect err but got %v", err)
	}

	if len(source.clusters[ns1])+len(source.clusters[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.clusters))
		t.Logf("target: %v", spew.Sdump(target.clusters))
		t.Fatal("should have deleted all capi clusters from the source k8s cluster")
	}
	if len(source.clusterResources[ns1])+len(source.clusterResources[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.clusterResources))
		t.Logf("target: %v", spew.Sdump(target.clusterResources))
		t.Fatal("should have deleted all capi cluster unstructured objects from the source k8s cluster")
	}
	if len(source.secrets[ns1])+len(source.secrets[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.secrets))
		t.Logf("target: %v", spew.Sdump(target.secrets))
		t.Fatal("should have deleted all capi cluster secrets from the source k8s cluster")
	}

	if len(source.machineDeployments[ns1])+len(source.machineDeployments[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.machineDeployments))
		t.Logf("target: %v", spew.Sdump(target.machineDeployments))
		t.Fatal("should have deleted all machine deployments from the source k8s cluster")
	}
	if len(source.machineSets[ns1])+len(source.machineSets[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.machineSets))
		t.Logf("target: %v", spew.Sdump(target.machineSets))
		t.Fatal("should have deleted all machine sets from source k8s cluster")
	}
	if len(source.machineTemplateResources[ns1])+len(source.machineTemplateResources[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.machineTemplateResources))
		t.Logf("target: %v", spew.Sdump(target.machineTemplateResources))
		t.Fatal("should have deleted all machine template unstructured objects from source k8s cluster")
	}

	if len(source.machines[ns1])+len(source.machines[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.machines))
		t.Logf("target: %v", spew.Sdump(target.machines))
		t.Fatal("should have deleted all machines from source k8s cluster")
	}
	if len(source.machineResources[ns1])+len(source.machineResources[ns2]) != 0 {
		t.Logf("source: %v", spew.Sdump(source.machineResources))
		t.Logf("target: %v", spew.Sdump(target.machineResources))
		t.Fatal("should have deleted all machine unstructured objects from source k8s cluster")
	}

	if len(target.clusters[ns1])+len(target.clusters[ns2]) != expectedClusters {
		t.Logf("source: %v", spew.Sdump(source.clusters))
		t.Logf("target: %v", spew.Sdump(target.clusters))
		t.Fatalf("expected %d clusters to pivot", expectedClusters)
	}
	if len(target.clusterResources[ns1])+len(target.clusterResources[ns2]) != expectedClusters {
		t.Logf("source: %v", spew.Sdump(source.clusterResources))
		t.Logf("target: %v", spew.Sdump(target.clusterResources))
		t.Fatalf("expected %d cluster unstructured objects to pivot", expectedClusters)
	}

	if len(target.machineDeployments[ns1])+len(target.machineDeployments[ns2]) != expectedMachineDeployments {
		t.Logf("source: %v", spew.Sdump(source.machineDeployments))
		t.Logf("target: %v", spew.Sdump(target.machineDeployments))
		t.Fatalf("expected %d machinedeployments for cluster to pivot", expectedMachineDeployments)
	}
	if len(target.machineSets[ns1])+len(target.machineSets[ns2]) != expectedMachineSets {
		t.Logf("source: %v", spew.Sdump(source.machineSets))
		t.Logf("target: %v", spew.Sdump(target.machineSets))
		for _, machineSets := range target.machineSets {
			for _, ms := range machineSets {
				t.Logf("machineSet: %v", ms)
			}
		}
		t.Fatalf("expected %d machinesets to pivot", expectedMachineSets)
	}
	if len(target.machineTemplateResources[ns1])+len(target.machineTemplateResources[ns2]) != expectedProviderMachineTemplates {
		t.Logf("source: %v", spew.Sdump(source.machineTemplateResources))
		t.Logf("target: %v", spew.Sdump(target.machineTemplateResources))
		t.Fatalf("expected %d machine template unstructured objects to pivot", expectedProviderMachineTemplates)
	}

	if len(target.machines[ns1])+len(target.machines[ns2]) != expectedMachines {
		t.Logf("source: %v", spew.Sdump(source.machines))
		t.Logf("target: %v", spew.Sdump(target.machines))
		for _, machines := range target.machines {
			for _, m := range machines {
				t.Logf("machine: %v", m)
			}
		}
		t.Fatalf("expected %d machines to pivot", expectedMachines)
	}
	if len(target.machineResources[ns1])+len(target.machineResources[ns2]) != expectedMachines {
		t.Logf("source: %v", spew.Sdump(source.machineResources))
		t.Logf("target: %v", spew.Sdump(target.machineResources))
		t.Fatalf("expected %d machine unstructured objects to pivot", expectedMachines)
	}
}

// An example of testing a failure scenario
// Override the function you want to fail with an embedded sourcer struct on a
// new type:
type waitFailSourcer struct {
	*sourcer
}

func (s *waitFailSourcer) WaitForClusterV1alpha2Ready() error {
	return errors.New("failed to wait for ready cluster resources")
}
func TestWaitForV1alpha2Failure(t *testing.T) {

	w := &waitFailSourcer{
		newSourcer(),
	}
	err := Pivot(w, newTarget(), "")
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
}
