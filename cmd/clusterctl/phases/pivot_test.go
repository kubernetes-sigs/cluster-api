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
	"errors"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// sourcer map keys are namespaces
// This is intentionally a different implementation than clusterclient
type sourcer struct {
	clusters           map[string][]*clusterv1.Cluster
	machineDeployments map[string][]*clusterv1.MachineDeployment
	machineSets        map[string][]*clusterv1.MachineSet
	machines           map[string][]*clusterv1.Machine
	machineClasses     map[string][]*clusterv1.MachineClass
}

func newSourcer() *sourcer {
	return &sourcer{
		clusters:           make(map[string][]*clusterv1.Cluster),
		machineDeployments: make(map[string][]*clusterv1.MachineDeployment),
		machineSets:        make(map[string][]*clusterv1.MachineSet),
		machines:           make(map[string][]*clusterv1.Machine),
		machineClasses:     make(map[string][]*clusterv1.MachineClass),
	}
}
func (s *sourcer) WithCluster(ns, name string) *sourcer {
	s.clusters[ns] = append(s.clusters[ns], &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
	}

	if cluster != "" {
		md.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster}
		blockOwnerDeletion := true
		md.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.SchemeGroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}
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
				APIVersion:         clusterv1.SchemeGroupVersion.Version,
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
				APIVersion: clusterv1.SchemeGroupVersion.Version,
				Kind:       "MachineDeployment",
				Name:       md,
				Controller: &isController,
			},
		}
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
	}
	if cluster != "" {
		m.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster}
		blockOwnerDeletion := true
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.SchemeGroupVersion.Version,
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
				APIVersion: clusterv1.SchemeGroupVersion.Version,
				Kind:       "MachineSet",
				Name:       ms,
				Controller: &isController,
			},
		}
	}

	s.machines[ns] = append(s.machines[ns], &m)
	return s
}

func (s *sourcer) WithMachineClass(ns, name string) *sourcer {
	s.machineClasses[ns] = append(s.machineClasses[ns], &clusterv1.MachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	})
	return s
}

// Interface implementation below

func (s *sourcer) Delete(string) error {
	return nil
}
func (s *sourcer) DeleteMachineClass(ns, name string) error {
	newMachineClasses := []*clusterv1.MachineClass{}
	for _, mc := range s.machineClasses[ns] {
		if mc.Name != name {
			newMachineClasses = append(newMachineClasses, mc)
		}
	}
	s.machineClasses[ns] = newMachineClasses
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
			for _, cluster := range clusters {
				out = append(out, cluster)
			}
		}
		return out, nil
	}
	return s.clusters[ns], nil
}
func (s *sourcer) GetMachineClasses(ns string) ([]*clusterv1.MachineClass, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.MachineClass{}
		for _, mcs := range s.machineClasses {
			for _, mc := range mcs {
				out = append(out, mc)
			}
		}
		return out, nil
	}
	return s.machineClasses[ns], nil
}
func (s *sourcer) GetMachineDeployments(ns string) ([]*clusterv1.MachineDeployment, error) {
	// empty ns implies all namespaces
	if ns == "" {
		out := []*clusterv1.MachineDeployment{}
		for _, mds := range s.machineDeployments {
			for _, md := range mds {
				out = append(out, md)
			}
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
			for _, m := range machines {
				out = append(out, m)
			}
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
			for _, ms := range machineSets {
				out = append(out, ms)
			}
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

func (s *sourcer) ScaleStatefulSet(string, string, int32) error {
	return nil
}
func (s *sourcer) WaitForClusterV1alpha1Ready() error {
	return nil
}

type target struct {
	clusters           map[string][]*clusterv1.Cluster
	machineDeployments map[string][]*clusterv1.MachineDeployment
	machineSets        map[string][]*clusterv1.MachineSet
	machines           map[string][]*clusterv1.Machine
	machineClasses     map[string][]*clusterv1.MachineClass
}

func (t *target) Apply(string) error {
	return nil
}
func (t *target) CreateClusterObject(c *clusterv1.Cluster) error {
	t.clusters[c.Namespace] = append(t.clusters[c.Namespace], c)
	return nil
}
func (t *target) CreateMachineClass(mc *clusterv1.MachineClass) error {
	t.machineClasses[mc.Namespace] = append(t.machineClasses[mc.Namespace], mc)
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

func (t *target) EnsureNamespace(string) error {
	return nil
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
func (t *target) WaitForClusterV1alpha1Ready() error {
	return nil
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
		WithMachineClass(ns1, "my-machine-class").
		WithMachineDeployment(ns1, "", "deployment2").
		WithMachineSet(ns1, "", "deployment2", "machineset4").
		WithMachine(ns1, "", "machineset4", "machine6").
		WithMachineSet(ns1, "", "", "machineset5").
		WithMachine(ns1, "", "machineset5", "machine7").
		WithMachine(ns1, "", "", "machine8").
		WithMachine(ns2, "", "", "machine9")

	expectedClusters := len(source.clusters[ns1]) + len(source.clusters[ns2])
	expectedMachineDeployments := len(source.machineDeployments[ns1]) + len(source.machineDeployments[ns2])
	expectedMachineSets := len(source.machineSets[ns1]) + len(source.machineSets[ns2])
	expectedMachines := len(source.machines[ns1]) + len(source.machines[ns2])
	expectedMachineClasses := len(source.machineClasses[ns1]) + len(source.machineClasses[ns2])

	target := &target{
		clusters:           make(map[string][]*clusterv1.Cluster),
		machineDeployments: make(map[string][]*clusterv1.MachineDeployment),
		machineSets:        make(map[string][]*clusterv1.MachineSet),
		machines:           make(map[string][]*clusterv1.Machine),
		machineClasses:     make(map[string][]*clusterv1.MachineClass),
	}

	if err := Pivot(source, target, pc.String()); err != nil {
		t.Fatalf("did not expect err but got %v", err)
	}

	if len(source.clusters[ns1])+len(source.clusters[ns2]) != 0 {
		t.Logf("source: %v", source.clusters)
		t.Logf("target: %v", target.clusters)
		t.Fatal("should have deleted all capi clusters from the source k8s cluster")
	}
	if len(source.machineDeployments[ns1])+len(source.machineDeployments[ns2]) != 0 {
		t.Logf("source: %v", source.machineDeployments)
		t.Logf("target: %v", target.machineDeployments)
		t.Fatal("should have deleted all machine deployments from the source k8s cluster")
	}
	if len(source.machineSets[ns1])+len(source.machineSets[ns2]) != 0 {
		t.Logf("source: %v", source.machineSets)
		t.Logf("target: %v", target.machineSets)
		t.Fatal("should have deleted all machine sets from source k8s cluster")
	}
	if len(source.machines[ns1])+len(source.machines[ns2]) != 0 {
		t.Logf("source: %v", source.machines)
		t.Logf("target: %v", target.machines)
		t.Fatal("should have deleted all machines from source k8s cluster")
	}
	if len(source.machineClasses[ns1])+len(source.machineClasses[ns2]) != 0 {
		t.Logf("source: %v", source.machineClasses)
		t.Logf("target: %v", target.machineClasses)
		t.Fatal("should have deleted all machine classes from source k8s cluster")
	}

	if len(target.clusters[ns1])+len(target.clusters[ns2]) != expectedClusters {
		t.Logf("source: %v", source.clusters)
		t.Logf("target: %v", target.clusters)
		t.Fatal("expected clusters to pivot")
	}
	if len(target.machineDeployments[ns1])+len(target.machineDeployments[ns2]) != expectedMachineDeployments {
		t.Logf("source: %v", source.machineDeployments)
		t.Logf("target: %v", target.machineDeployments)
		t.Fatal("expected machinedeployments for cluster to pivot")
	}
	if len(target.machineSets[ns1])+len(target.machineSets[ns2]) != expectedMachineSets {
		t.Logf("source: %v", source.machineSets)
		t.Logf("target: %v", target.machineSets)
		for _, machineSets := range target.machineSets {
			for _, ms := range machineSets {
				t.Logf("machineSet: %v", ms)
			}
		}
		t.Fatal("expected machines sets to pivot")
	}
	if len(target.machines[ns1])+len(target.machines[ns2]) != expectedMachines {
		t.Logf("source: %v", source.machines)
		t.Logf("target: %v", target.machines)
		for _, machines := range target.machines {
			for _, m := range machines {
				t.Logf("machine: %v", m)
			}
		}
		t.Fatal("expected machines to pivot")
	}
	if len(target.machineClasses[ns1])+len(target.machineClasses[ns2]) != expectedMachineClasses {
		t.Logf("source: %v", source.machineClasses)
		t.Logf("target: %v", target.machineClasses)
		t.Fatal("expected machine classes to pivot")
	}
}

// An example of testing a failure scenario
// Override the function you want to fail with an embedded sourcer struct on a
// new type:
type waitFailSourcer struct {
	*sourcer
}

func (s *waitFailSourcer) WaitForClusterV1alpha1Ready() error {
	return errors.New("failed to wait for ready cluster resources")
}
func TestWaitForV1alpha1Failure(t *testing.T) {

	w := &waitFailSourcer{
		newSourcer(),
	}
	err := Pivot(w, &target{}, "")
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
}
