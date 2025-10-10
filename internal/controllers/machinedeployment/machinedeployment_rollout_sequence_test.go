/*
Copyright 2025 The Kubernetes Authors.

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

package machinedeployment

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

// machineControllerMutator fakes a small part of the Machine controller, just what is required for the rollout to progress.
func machineControllerMutator(log *fileLogger, m *clusterv1.Machine, scope *rolloutScope) {
	if m.DeletionTimestamp.IsZero() {
		return
	}

	log.Logf("[M controller] - %s finalizer removed", m.Name)
	ms := m.OwnerReferences[0].Name
	machinesSetMachines := []*clusterv1.Machine{}
	for _, mx := range scope.machineSetMachines[ms] {
		if mx.Name == m.Name {
			continue
		}
		machinesSetMachines = append(machinesSetMachines, mx)
	}
	scope.machineSetMachines[ms] = machinesSetMachines
}

// machineSetControllerMutator fakes a small part of the MachineSet controller, just what is required for the rollout to progress.
func machineSetControllerMutator(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope) {
	// Update counters
	// Note: this should not be implemented in production code
	ms.Status.Replicas = ptr.To(int32(len(scope.machineSetMachines[ms.Name])))

	// Sort machines to ensure stable results of move/delete operations during tests.
	// Note: this should not be implemented in production code
	sortMachineSetMachines(scope.machineSetMachines[ms.Name])

	// if too few machines, create missing machine.
	// new machines are created with a predictable name, so it is easier to write test case and validate rollout sequences.
	// e.g. if the cluster is initialized with m1, m2, m3, new machines will be m4, m5, m6
	if _, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; !ok {
		machinesToAdd := ptr.Deref(ms.Spec.Replicas, 0) - ptr.Deref(ms.Status.Replicas, 0)
		if machinesToAdd > 0 {
			machinesAdded := []string{}
			for range machinesToAdd {
				machineName := fmt.Sprintf("m%d", scope.GetNextMachineUID())
				scope.machineSetMachines[ms.Name] = append(scope.machineSetMachines[ms.Name],
					&clusterv1.Machine{
						ObjectMeta: metav1.ObjectMeta{
							Name: machineName,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "MachineSet",
									Name:       ms.Name,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: *ms.Spec.Template.Spec.DeepCopy(),
					},
				)
				machinesAdded = append(machinesAdded, machineName)
			}

			log.Logf("[MS controller] - %s scale up to %d/%[2]d replicas (%s created)", ms.Name, ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesAdded, ","))
		}
	}

	// if too many replicas, delete exceeding machines.
	// exceeding machines are deleted in predictable order, so it is easier to write test case and validate rollout sequences.
	// e.g. if a ms has m1,m2,m3 created in this order, m1 will be deleted first, then m2 and finally m3.
	machinesToDelete := max(ptr.Deref(ms.Status.Replicas, 0)-ptr.Deref(ms.Spec.Replicas, 0), 0)

	if machinesToDelete > 0 {
		machinesDeleted := []string{}
		machinesSetMachines := []*clusterv1.Machine{}
		for i, m := range scope.machineSetMachines[ms.Name] {
			if int32(len(machinesDeleted)) >= machinesToDelete {
				machinesSetMachines = append(machinesSetMachines, scope.machineSetMachines[ms.Name][i:]...)
				break
			}
			machinesDeleted = append(machinesDeleted, m.Name)
		}
		scope.machineSetMachines[ms.Name] = machinesSetMachines
		log.Logf("[MS controller] - %s scale down to %d/%[2]d replicas (%s deleted)", ms.Name, ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesDeleted, ","))
	}

	// Update counters
	ms.Status.Replicas = ptr.To(int32(len(scope.machineSetMachines[ms.Name])))
	ms.Status.AvailableReplicas = ptr.To(int32(len(scope.machineSetMachines[ms.Name])))
}

type rolloutScope struct {
	machineDeployment  *clusterv1.MachineDeployment
	machineSets        []*clusterv1.MachineSet
	machineSetMachines map[string][]*clusterv1.Machine

	machineUID int32
}

func initCurrentRolloutScope(currentMachineNames []string, mdOptions ...machineDeploymentOption) (current *rolloutScope) {
	// create current state, with a MD with
	// - given MaxSurge, MaxUnavailable
	// - replica counters assuming all the machines are at stable state
	// - spec different from the MachineSets and Machines we are going to create down below (to simulate a change that triggers a rollout, but it is not yet started)
	mdReplicaCount := int32(len(currentMachineNames))
	current = &rolloutScope{
		machineDeployment: createMD("v2", mdReplicaCount, mdOptions...),
	}

	// Create current MS, with
	// - replica counters assuming all the machines are at stable state
	// - spec at stable state (rollout is not yet propagated to machines)
	ms := createMS("ms1", "v1", mdReplicaCount)
	current.machineSets = append(current.machineSets, ms)

	// Create current Machines, with
	// - spec at stable state (rollout is not yet propagated to machines)
	var totMachines int32
	currentMachines := []*clusterv1.Machine{}
	for _, machineSetMachineName := range currentMachineNames {
		totMachines++
		currentMachines = append(currentMachines, createM(machineSetMachineName, ms.Name, ms.Spec.Template.Spec.FailureDomain))
	}
	current.machineSetMachines = map[string][]*clusterv1.Machine{}
	current.machineSetMachines[ms.Name] = currentMachines
	current.machineUID = totMachines

	// TODO(in-place): this should be removed as soon as rolloutPlanner will take care of creating newMS
	newMS := createMS("ms2", current.machineDeployment.Spec.Template.Spec.FailureDomain, 0)
	current.machineSets = append(current.machineSets, newMS)

	return current
}

func computeDesiredRolloutScope(current *rolloutScope, desiredMachineNames []string) (desired *rolloutScope) {
	var totMachineSets, totMachines int32
	totMachineSets = int32(len(current.machineSets))
	for _, msMachines := range current.machineSetMachines {
		totMachines += int32(len(msMachines))
	}

	// Create current state, with a MD equal to the one we started from because:
	// - spec was already changed in current to simulate a change that triggers a rollout
	// - desired replica counters are the same than current replica counters (we start with all the machines at stable state v1, we should end with all the machines at stable state v2)
	desired = &rolloutScope{
		machineDeployment: current.machineDeployment.DeepCopy(),
	}
	desired.machineDeployment.Status.Replicas = desired.machineDeployment.Spec.Replicas
	desired.machineDeployment.Status.AvailableReplicas = desired.machineDeployment.Spec.Replicas

	// Add current MS to desired state, but set replica counters to zero because all the machines must be moved to the new MS.
	// Note: one of the old MS could also be the NewMS, the MS that must become owner of all the desired machines.
	var newMS *clusterv1.MachineSet
	for _, currentMS := range current.machineSets {
		oldMS := currentMS.DeepCopy()
		oldMS.Spec.Replicas = ptr.To(int32(0))
		oldMS.Status.Replicas = ptr.To(int32(0))
		oldMS.Status.AvailableReplicas = ptr.To(int32(0))
		desired.machineSets = append(desired.machineSets, oldMS)

		if upToDate, _ := mdutil.MachineTemplateUpToDate(&oldMS.Spec.Template, &desired.machineDeployment.Spec.Template); upToDate {
			if newMS != nil {
				panic("there should be only one MachineSet with MachineTemplateUpToDate")
			}
			newMS = oldMS
		}
	}

	// Add or update the new MS to desired state, with
	// - the new spec from the MD
	// - replica counters assuming all the replicas must be here at the end of the rollout.
	if newMS != nil {
		newMS.Spec.Replicas = desired.machineDeployment.Spec.Replicas
		newMS.Status.Replicas = desired.machineDeployment.Status.Replicas
		newMS.Status.AvailableReplicas = desired.machineDeployment.Status.AvailableReplicas
	} else {
		totMachineSets++
		newMS = createMS(fmt.Sprintf("ms%d", totMachineSets), desired.machineDeployment.Spec.Template.Spec.FailureDomain, *desired.machineDeployment.Spec.Replicas)
		desired.machineSets = append(desired.machineSets, newMS)
	}

	// Add a desired machines to desired state, with
	// - the new spec from the MD (steady state)
	desiredMachines := []*clusterv1.Machine{}
	for _, machineSetMachineName := range desiredMachineNames {
		totMachines++
		desiredMachines = append(desiredMachines, createM(machineSetMachineName, newMS.Name, newMS.Spec.Template.Spec.FailureDomain))
	}
	desired.machineSetMachines = map[string][]*clusterv1.Machine{}
	desired.machineSetMachines[newMS.Name] = desiredMachines
	return desired
}

// GetNextMachineUID provides a predictable UID for machines.
func (r *rolloutScope) GetNextMachineUID() int32 {
	r.machineUID++
	return r.machineUID
}

func (r *rolloutScope) Clone() *rolloutScope {
	if r == nil {
		return nil
	}

	c := &rolloutScope{
		machineDeployment:  r.machineDeployment.DeepCopy(),
		machineSetMachines: map[string][]*clusterv1.Machine{},
		machineUID:         r.machineUID,
	}
	for _, ms := range r.machineSets {
		c.machineSets = append(c.machineSets, ms.DeepCopy())
	}
	for ms, machines := range r.machineSetMachines {
		cmachines := make([]*clusterv1.Machine, 0, len(machines))
		for _, m := range machines {
			cmachines = append(cmachines, m.DeepCopy())
		}
		c.machineSetMachines[ms] = cmachines
	}
	return c
}

func (r rolloutScope) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%s, %d/%d replicas\n", r.machineDeployment.Name, ptr.Deref(r.machineDeployment.Status.Replicas, 0), ptr.Deref(r.machineDeployment.Spec.Replicas, 0)))

	sort.Slice(r.machineSets, func(i, j int) bool { return r.machineSets[i].Name < r.machineSets[j].Name })
	for _, ms := range r.machineSets {
		sb.WriteString(fmt.Sprintf("- %s, %s\n", ms.Name, msLog(ms, r.machineSetMachines[ms.Name])))
	}
	return sb.String()
}

func msLog(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) string {
	sb := strings.Builder{}
	machineNames := []string{}
	for _, m := range machines {
		machineNames = append(machineNames, m.Name)
	}
	sb.WriteString(strings.Join(machineNames, ","))
	msLog := fmt.Sprintf("%d/%d replicas (%s)", ptr.Deref(ms.Status.Replicas, 0), ptr.Deref(ms.Spec.Replicas, 0), sb.String())
	return msLog
}

func (r rolloutScope) newMS() *clusterv1.MachineSet {
	for _, ms := range r.machineSets {
		if upToDate, _ := mdutil.MachineTemplateUpToDate(&r.machineDeployment.Spec.Template, &ms.Spec.Template); upToDate {
			return ms
		}
	}
	return nil
}

func (r rolloutScope) oldMSs() []*clusterv1.MachineSet {
	var oldMSs []*clusterv1.MachineSet
	for _, ms := range r.machineSets {
		if upToDate, _ := mdutil.MachineTemplateUpToDate(&r.machineDeployment.Spec.Template, &ms.Spec.Template); !upToDate {
			oldMSs = append(oldMSs, ms)
		}
	}
	return oldMSs
}

func (r rolloutScope) machines() []*clusterv1.Machine {
	machines := []*clusterv1.Machine{}
	for _, ms := range r.machineSets {
		machines = append(machines, r.machineSetMachines[ms.Name]...)
	}
	return machines
}

func (r *rolloutScope) Equal(s *rolloutScope) bool {
	return machineDeploymentIsEqual(r.machineDeployment, s.machineDeployment) && machineSetsAreEqual(r.machineSets, s.machineSets) && machineSetMachinesAreEqual(r.machineSetMachines, s.machineSetMachines)
}

func machineDeploymentIsEqual(a, b *clusterv1.MachineDeployment) bool {
	if upToDate, _ := mdutil.MachineTemplateUpToDate(&a.Spec.Template, &b.Spec.Template); !upToDate ||
		ptr.Deref(a.Spec.Replicas, 0) != ptr.Deref(b.Spec.Replicas, 0) ||
		ptr.Deref(a.Status.Replicas, 0) != ptr.Deref(b.Status.Replicas, 0) ||
		ptr.Deref(a.Status.AvailableReplicas, 0) != ptr.Deref(b.Status.AvailableReplicas, 0) {
		return false
	}
	return true
}

func machineSetsAreEqual(a, b []*clusterv1.MachineSet) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]*clusterv1.MachineSet)
	for i := range a {
		aMap[a[i].Name] = a[i]
	}

	for i := range b {
		desiredMS := b[i]
		currentMS, ok := aMap[desiredMS.Name]
		if !ok {
			return false
		}
		if upToDate, _ := mdutil.MachineTemplateUpToDate(&desiredMS.Spec.Template, &currentMS.Spec.Template); !upToDate ||
			ptr.Deref(desiredMS.Spec.Replicas, 0) != ptr.Deref(currentMS.Spec.Replicas, 0) ||
			ptr.Deref(desiredMS.Status.Replicas, 0) != ptr.Deref(currentMS.Status.Replicas, 0) ||
			ptr.Deref(desiredMS.Status.AvailableReplicas, 0) != ptr.Deref(currentMS.Status.AvailableReplicas, 0) {
			return false
		}
	}
	return true
}

func machineSetMachinesAreEqual(a, b map[string][]*clusterv1.Machine) bool {
	for ms, aMachines := range a {
		bMachines, ok := b[ms]
		if !ok {
			if len(aMachines) > 0 {
				return false
			}
			continue
		}

		if len(aMachines) != len(bMachines) {
			return false
		}

		for i := range aMachines {
			if aMachines[i].Name != bMachines[i].Name {
				return false
			}
			if len(aMachines[i].OwnerReferences) != 1 || len(bMachines[i].OwnerReferences) != 1 || aMachines[i].OwnerReferences[0].Name != bMachines[i].OwnerReferences[0].Name {
				return false
			}
		}
	}
	return true
}

type UniqueRand struct {
	rng       *rand.Rand
	generated map[int]bool // keeps track of random numbers already generated.
	max       int          // max number to be generated
}

func (u *UniqueRand) Int() int {
	if u.Done() {
		return -1
	}
	for {
		i := u.rng.Intn(u.max)
		if !u.generated[i] {
			u.generated[i] = true
			return i
		}
	}
}

func (u *UniqueRand) Done() bool {
	return len(u.generated) >= u.max
}

func (u *UniqueRand) Forget(n int) {
	delete(u.generated, n)
}

type fileLogger struct {
	t *testing.T

	testCase              string
	fileName              string
	testCaseStringBuilder strings.Builder
	writeGoldenFile       bool
}

func newFileLogger(t *testing.T, name, fileName string) *fileLogger {
	t.Helper()

	l := &fileLogger{t: t, testCaseStringBuilder: strings.Builder{}}
	l.testCaseStringBuilder.WriteString(fmt.Sprintf("## %s\n\n", name))
	l.testCase = name
	l.fileName = fileName
	return l
}

func (l *fileLogger) Logf(format string, args ...interface{}) {
	l.t.Logf(format, args...)

	// this codes takes a log line that has been formatted for t.Logf and change it
	// so it will look nice in the files. e.g. adds indentation to all lines except the fist one, which by convention starts with [.
	s := strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
	sb := &strings.Builder{}
	if strings.Contains(s, "\n") {
		lines := strings.Split(s, "\n")
		for _, line := range lines {
			indent := "  "
			if strings.HasPrefix(line, "[") {
				indent = ""
			}
			sb.WriteString(indent + line + "\n")
		}
	} else {
		sb.WriteString(s + "\n")
	}
	l.testCaseStringBuilder.WriteString(sb.String())
}

func (l *fileLogger) WriteLogAndCompareWithGoldenFile() (string, string, error) {
	if err := os.WriteFile(fmt.Sprintf("%s.test.log", l.fileName), []byte(l.testCaseStringBuilder.String()), 0600); err != nil {
		return "", "", err
	}
	if l.writeGoldenFile {
		if err := os.WriteFile(fmt.Sprintf("%s.test.log.golden", l.fileName), []byte(l.testCaseStringBuilder.String()), 0600); err != nil {
			return "", "", err
		}
	}

	currentBytes, _ := os.ReadFile(fmt.Sprintf("%s.test.log", l.fileName))
	current := string(currentBytes)

	goldenBytes, _ := os.ReadFile(fmt.Sprintf("%s.test.log.golden", l.fileName))
	golden := string(goldenBytes)

	return current, golden, nil
}

func sortMachineSetMachines(machines []*clusterv1.Machine) {
	sort.Slice(machines, func(i, j int) bool {
		iIndex, _ := strconv.Atoi(strings.TrimPrefix(machines[i].Name, "m"))
		jiIndex, _ := strconv.Atoi(strings.TrimPrefix(machines[j].Name, "m"))
		return iIndex < jiIndex
	})
}

// default task order ensure the controllers are run in a consistent and predictable way: md, ms1, ms2 and so on.
func defaultTaskOrder(taskCount int) []int {
	taskOrder := []int{}
	for t := range taskCount {
		taskOrder = append(taskOrder, t)
	}
	return taskOrder
}

func randomTaskOrder(taskCount int, rng *rand.Rand) []int {
	u := &UniqueRand{
		rng:       rng,
		generated: map[int]bool{},
		max:       taskCount,
	}
	taskOrder := []int{}
	for !u.Done() {
		n := u.Int()
		if rng.Intn(10) < 3 { // skip a step in the 30% of cases
			continue
		}
		taskOrder = append(taskOrder, n)
		if r := rng.Intn(10); r < 3 { // repeat a step in the 30% of cases
			u.Forget(n)
		}
	}
	return taskOrder
}

type machineDeploymentOption func(md *clusterv1.MachineDeployment)

func withRollingUpdateStrategy(maxSurge, maxUnavailable int32) func(md *clusterv1.MachineDeployment) {
	return func(md *clusterv1.MachineDeployment) {
		md.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
				MaxSurge:       ptr.To(intstr.FromInt32(maxSurge)),
				MaxUnavailable: ptr.To(intstr.FromInt32(maxUnavailable)),
			},
		}
	}
}

func withOnDeleteStrategy() func(md *clusterv1.MachineDeployment) {
	return func(md *clusterv1.MachineDeployment) {
		md.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
		}
	}
}

func createMD(failureDomain string, replicas int32, options ...machineDeploymentOption) *clusterv1.MachineDeployment {
	md := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "md"},
		Spec: clusterv1.MachineDeploymentSpec{
			// Note: using failureDomain as a template field to determine upToDate
			Template: clusterv1.MachineTemplateSpec{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			Replicas: &replicas,
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          &replicas,
			AvailableReplicas: &replicas,
		},
	}
	for _, opt := range options {
		opt(md)
	}
	return md
}

func withStatusAvailableReplicas(r int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.AvailableReplicas = ptr.To(r)
	}
}

func createMS(name, failureDomain string, replicas int32, opts ...fakeMachineSetOption) *clusterv1.MachineSet {
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.MachineSetSpec{
			// Note: using failureDomain as a template field to determine upToDate
			Template: clusterv1.MachineTemplateSpec{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			Replicas: ptr.To(replicas),
		},
		Status: clusterv1.MachineSetStatus{
			Replicas:          ptr.To(replicas),
			AvailableReplicas: ptr.To(replicas),
		},
	}
	for _, opt := range opts {
		opt(ms)
	}
	return ms
}

func createM(name, ownedByMS, failureDomain string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       ownedByMS,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			// Note: using failureDomain as a template field to determine upToDate
			FailureDomain: failureDomain,
		},
	}
}
