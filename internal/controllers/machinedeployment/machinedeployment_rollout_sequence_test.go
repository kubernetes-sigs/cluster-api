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
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

type rolloutSequenceTestCase struct {
	name           string
	maxSurge       int32
	maxUnavailable int32

	// currentMachineNames is the list of machines before the rollout, and provides a simplified alternative to currentScope.
	// all the machines in this list are initialized as upToDate and owned by the new MS before the rollout (which is different from the new MS after the rollout).
	// Please name machines as "mX" where X is a progressive number starting from 1 (do not skip numbers),
	// e.g. "m1","m2","m3"
	currentMachineNames []string

	// currentScope defines the current state at the beginning of the test case.
	// When the test case start from a stable state (there are no previous rollout in progress), use  currentMachineNames instead.
	// Please name machines as "mX" where X is a progressive number starting from 1 (do not skip numbers),
	// e.g. "m1","m2","m3"
	// machineUID must be set to the last used number.
	currentScope *rolloutScope

	// maxUnavailableBreachToleration can be used to temporarily silence MaxUnavailable breaches
	//
	// maxUnavailableBreachToleration: func(log *logger, i int, scope *rolloutScope, minAvailableReplicas, totAvailableReplicas int32) bool {
	// 		if i == 5 {
	// 			t.Log("[Toleration] tolerate minAvailable breach after scale up")
	// 			return true
	// 		}
	// 		return false
	// 	},
	maxUnavailableBreachToleration func(log *fileLogger, i int, scope *rolloutScope, minAvailableReplicas, totAvailableReplicas int32) bool

	// maxSurgeBreachToleration can be used to temporarily silence MaxSurge breaches
	// (see maxUnavailableBreachToleration example)
	maxSurgeBreachToleration func(log *fileLogger, i int, scope *rolloutScope, maxAllowedReplicas, totReplicas int32) bool

	// desiredMachineNames is the list of machines at the end of the rollout.
	// all the machines in this list are expected to be upToDate and owned by the new MS after the rollout (which is different from the new MS before the rollout).
	// if this list contains old machines names (machine names already in currentMachineNames), it implies those machine have been upgraded in places.
	// if this list contains new machines names (machine names not in currentMachineNames), it implies those machines have been created during a rollout;
	// please name new machines names as "mX" where X is a progressive number starting after the max number in currentMachineNames (do not skip numbers),
	// e.g. desiredMachineNames "m4","m5","m6" (desired machine names after a regular rollout of a MD with currentMachineNames "m1","m2","m3")
	// e.g. desiredMachineNames "m1","m2","m3" (desired machine names after rollout performed using in-place upgrade for an MD with currentMachineNames "m1","m2","m3")
	desiredMachineNames []string

	// skipLogToFileAndGoldenFileCheck allows to skip storing the log to file and golden file Check.
	// NOTE: this field is controlled by the test itself.
	skipLogToFileAndGoldenFileCheck bool

	// name of the log to file and the golden file.
	// NOTE: this field is controlled by the test itself.
	logAndGoldenFileName string

	// randomControllerOrder force the tests to run controllers in random order, mimicking what happens in production.
	// NOTE. We are using a pseudo randomizer, so the random order remains consistent across runs of the same groups of tests.
	// NOTE: this field is controlled by the test itself.
	randomControllerOrder bool

	// maxIterations defines the max number of iterations the system must attempt before assuming the logic has an issue
	// in reaching the desired state.
	// When the test is using default controller order, an iteration implies reconcile MD + reconcile all MS in a predictable order;
	// while using randomControllerOrder the concept of iteration is less defined, but it can still be used to prevent
	// the test from running indefinitely.
	// NOTE: this field is controlled by the test itself.
	maxIterations int

	// seed value to initialize the generator.
	// NOTE: this field is controlled by the test itself.
	seed int64
}

func Test_rolloutSequencesWithPredictableReconcileOrder(t *testing.T) {
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, klog.Background())

	tests := []rolloutSequenceTestCase{
		// Regular rollout (no in-place)

		{ // scale out by 1
			name:                "Regular rollout, 3 Replicas, maxSurge 1, maxUnavailable 0",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m4", "m5", "m6"},
		},
		{ // scale in by 1
			name:                "Regular rollout, 3 Replicas, maxSurge 0, maxUnavailable 1",
			maxSurge:            0,
			maxUnavailable:      1,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m4", "m5", "m6"},
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1",
			maxSurge:            3,
			maxUnavailable:      1,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                "Regular rollout, 6 Replicas, maxSurge 1, maxUnavailable 3",
			maxSurge:            1,
			maxUnavailable:      3,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                "Regular rollout, 6 Replicas, maxSurge 10, maxUnavailable 0",
			maxSurge:            10,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                "Regular rollout, 6 Replicas, maxSurge 0, maxUnavailable 10",
			maxSurge:            0,
			maxUnavailable:      10,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale up machine deployment in the middle
			name:           "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1, scale up to 12",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD scaled up to 12.
				machineDeployment: createMD("v2", 12, 3, 1),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						createM("m7", "ms2", "v2"),
						createM("m8", "ms2", "v2"),
						createM("m9", "ms2", "v2"),
					},
				},
				machineUID: 9,
			},
			desiredMachineNames:            []string{"m7", "m8", "m9", "m10", "m11", "m12", "m13", "m14", "m15", "m16", "m17", "m18"},
			maxUnavailableBreachToleration: maxUnavailableBreachToleration(),
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale down machine deployment in the middle
			name:           "Regular rollout, 12 Replicas, maxSurge 3, maxUnavailable 1, scale down to 6",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 12 replica in the middle of a rollout, with 3 machines already created in the newMS and 9 still on the oldMS, and then MD scaled down to 6.
				machineDeployment: createMD("v2", 6, 3, 1),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 9),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
						createM("m7", "ms1", "v1"),
						createM("m8", "ms1", "v1"),
						createM("m9", "ms1", "v1"),
						createM("m10", "ms1", "v1"),
						createM("m11", "ms1", "v1"),
						createM("m12", "ms1", "v1"),
					},
					"ms2": {
						createM("m13", "ms2", "v2"),
						createM("m14", "ms2", "v2"),
						createM("m15", "ms2", "v2"),
					},
				},
				machineUID: 15,
			},
			desiredMachineNames:      []string{"m13", "m14", "m15", "m16", "m17", "m18"},
			maxSurgeBreachToleration: maxSurgeToleration(),
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + change spec in the middle
			name:           "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1, change spec",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD spec is changed.
				machineDeployment: createMD("v3", 6, 3, 1),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
					createMS("ms3", "v3", 0),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						createM("m7", "ms2", "v2"),
						createM("m8", "ms2", "v2"),
						createM("m9", "ms2", "v2"),
					},
				},
				machineUID: 9,
			},
			desiredMachineNames: []string{"m10", "m11", "m12", "m13", "m14", "m15"}, // NOTE: Machines created before the spec change are deleted
		},
	}

	testWithPredictableReconcileOrder := true
	// TODO(in-place): enable tests with random reconcile order as soon as the issues in reconcileOldMachineSets are fixed
	testWithRandomReconcileOrderFromConstantSeed := false
	testWithRandomReconcileOrderFromRandomSeed := false

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.name

			if testWithPredictableReconcileOrder {
				tt.maxIterations = 50
				tt.randomControllerOrder = false
				if tt.logAndGoldenFileName == "" {
					tt.logAndGoldenFileName = strings.ToLower(tt.name)
				}
				t.Run("default", func(t *testing.T) {
					runTestCase(ctx, t, tt)
				})
			}

			if testWithRandomReconcileOrderFromConstantSeed {
				tt.maxIterations = 70
				tt.name = fmt.Sprintf("%s, random(0)", name)
				tt.randomControllerOrder = true
				tt.seed = 0
				// TODO(in-place): drop the following line as soon as issue with scale down are fixed
				tt.skipLogToFileAndGoldenFileCheck = true
				if tt.logAndGoldenFileName == "" {
					tt.logAndGoldenFileName = strings.ToLower(tt.name)
				}
				t.Run("random(0)", func(t *testing.T) {
					runTestCase(ctx, t, tt)
				})
			}

			if testWithRandomReconcileOrderFromRandomSeed {
				for range 100 {
					tt.maxIterations = 150
					tt.seed = time.Now().UnixNano()
					tt.name = fmt.Sprintf("%s, random(%d)", name, tt.seed)
					tt.randomControllerOrder = true
					tt.skipLogToFileAndGoldenFileCheck = true
					t.Run(fmt.Sprintf("random(%d)", tt.seed), func(t *testing.T) {
						runTestCase(ctx, t, tt)
					})
				}
			}
		})
	}
}

func runTestCase(ctx context.Context, t *testing.T, tt rolloutSequenceTestCase) {
	t.Helper()
	g := NewWithT(t)

	rng := rand.New(rand.NewSource(tt.seed)) //nolint:gosec // it is ok to use a weak randomizer here
	fLogger := newFileLogger(t, tt.name, fmt.Sprintf("testdata/%s", tt.logAndGoldenFileName))

	// Init current and desired state from test case
	current := tt.currentScope.Clone()
	if current == nil {
		current = initCurrentRolloutScope(tt)
	}
	desired := computeDesiredRolloutScope(current, tt.desiredMachineNames)

	// Log initial state
	fLogger.Logf("[Test] Initial state\n%s", current)
	random := ""
	if tt.randomControllerOrder {
		random = fmt.Sprintf(", random(%d)", tt.seed)
	}
	fLogger.Logf("[Test] Rollout %d replicas, MaxSurge=%d, MaxUnavailable=%d%s\n", len(tt.currentMachineNames), tt.maxSurge, tt.maxUnavailable, random)
	i := 1
	maxIterations := tt.maxIterations
	for {
		taskOrder := defaultTaskOrder(current)
		if tt.randomControllerOrder {
			taskOrder = randomTaskOrder(current, rng)
		}
		for _, taskID := range taskOrder {
			if taskID == 0 {
				fLogger.Logf("[MD controller] Iteration %d, Reconcile md", i)
				fLogger.Logf("[MD controller] - Input to rollout planner\n%s", current)

				// Running a small subset of MD reconcile (the rollout logic and a bit of setReplicas)
				p := newRolloutPlanner()
				p.md = current.machineDeployment
				p.newMS = current.newMS()
				p.oldMSs = current.oldMSs()

				err := p.Plan(ctx)
				g.Expect(err).ToNot(HaveOccurred())
				// Apply changes.
				for _, ms := range current.machineSets {
					if scaleIntent, ok := p.scaleIntents[ms.Name]; ok {
						ms.Spec.Replicas = ptr.To(scaleIntent)
					}
				}

				// Running a small subset of setReplicas (we don't want to run the full func to avoid unnecessary noise on the test)
				current.machineDeployment.Status.Replicas = mdutil.GetActualReplicaCountForMachineSets(current.machineSets)
				current.machineDeployment.Status.AvailableReplicas = mdutil.GetAvailableReplicaCountForMachineSets(current.machineSets)

				// Log state after this reconcile
				fLogger.Logf("[MD controller] - Result of rollout planner\n%s", current)

				// Check we are not breaching rollout constraints
				minAvailableReplicas := ptr.Deref(current.machineDeployment.Spec.Replicas, 0) - mdutil.MaxUnavailable(*current.machineDeployment)
				totAvailableReplicas := ptr.Deref(current.machineDeployment.Status.AvailableReplicas, 0)
				if totAvailableReplicas < minAvailableReplicas {
					tolerateBreach := false
					if tt.maxUnavailableBreachToleration != nil {
						tolerateBreach = tt.maxUnavailableBreachToleration(fLogger, i, current, minAvailableReplicas, totAvailableReplicas)
					}
					if !tolerateBreach {
						g.Expect(totAvailableReplicas).To(BeNumerically(">=", minAvailableReplicas), "totAvailable machines is less than md.spec.replicas - maxUnavailable")
					}
				}

				maxAllowedReplicas := ptr.Deref(current.machineDeployment.Spec.Replicas, 0) + mdutil.MaxSurge(*current.machineDeployment)
				totReplicas := mdutil.TotalMachineSetsReplicaSum(current.machineSets)
				if totReplicas > maxAllowedReplicas {
					tolerateBreach := false
					if tt.maxSurgeBreachToleration != nil {
						tolerateBreach = tt.maxSurgeBreachToleration(fLogger, i, current, maxAllowedReplicas, totReplicas)
					}
					if !tolerateBreach {
						g.Expect(totReplicas).To(BeNumerically("<=", maxAllowedReplicas), "totReplicas machines is greater than md.spec.replicas + maxSurge")
					}
				}
			}

			// Run mutators faking other controllers
			for _, ms := range current.machineSets {
				if fmt.Sprintf("ms%d", taskID) == ms.Name {
					fLogger.Logf("[MS controller] Iteration %d, Reconcile ms%d, %s", i, taskID, msLog(ms, current.machineSetMachines[ms.Name]))
					machineSetControllerMutator(fLogger, ms, current)
					break
				}
			}
		}

		// Check if we are at the desired state
		if current.Equal(desired) {
			fLogger.Logf("[Test] Final state\n%s", current)
			break
		}

		// Safeguard for infinite reconcile
		i++
		if i > maxIterations {
			current.Equal(desired)
			// Log desired state we never reached
			fLogger.Logf("[Test] Desired state\n%s", desired)
			g.Fail(fmt.Sprintf("Failed to reach desired state in %d iterations", maxIterations))
		}
	}

	if !tt.skipLogToFileAndGoldenFileCheck {
		currentLog, goldenLog, err := fLogger.WriteLogAndCompareWithGoldenFile()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentLog).To(Equal(goldenLog), "current test case log and golden test case log are different\n%s", cmp.Diff(currentLog, goldenLog))
	}
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
				},
			)
			machinesAdded = append(machinesAdded, machineName)
		}

		log.Logf("[MS controller] - %s scale up to %d/%[2]d replicas (%s created)", ms.Name, ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesAdded, ","))
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

// Init creates current state and desired state for rolling out a md from currentMachines to wantMachineNames.
func initCurrentRolloutScope(tt rolloutSequenceTestCase) (current *rolloutScope) {
	// create current state, with a MD with
	// - given MaxSurge, MaxUnavailable
	// - replica counters assuming all the machines are at stable state
	// - spec different from the MachineSets and Machines we are going to create down below (to simulate a change that triggers a rollout, but it is not yet started)
	mdReplicaCount := int32(len(tt.currentMachineNames))
	current = &rolloutScope{
		machineDeployment: createMD("v2", mdReplicaCount, tt.maxSurge, tt.maxUnavailable),
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
	for _, machineSetMachineName := range tt.currentMachineNames {
		totMachines++
		currentMachines = append(currentMachines, createM(machineSetMachineName, ms.Name, ms.Spec.Template.Spec.FailureDomain))
	}
	current.machineSetMachines = map[string][]*clusterv1.Machine{}
	current.machineSetMachines[ms.Name] = currentMachines

	current.machineDeployment.Spec.Replicas = ptr.To(mdReplicaCount)
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

		if upToDate, _, _ := mdutil.MachineTemplateUpToDate(&oldMS.Spec.Template, &desired.machineDeployment.Spec.Template); upToDate {
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
		if upToDate, _, _ := mdutil.MachineTemplateUpToDate(&r.machineDeployment.Spec.Template, &ms.Spec.Template); upToDate {
			return ms
		}
	}
	return nil
}

func (r rolloutScope) oldMSs() []*clusterv1.MachineSet {
	var oldMSs []*clusterv1.MachineSet
	for _, ms := range r.machineSets {
		if upToDate, _, _ := mdutil.MachineTemplateUpToDate(&r.machineDeployment.Spec.Template, &ms.Spec.Template); !upToDate {
			oldMSs = append(oldMSs, ms)
		}
	}
	return oldMSs
}

func (r *rolloutScope) Equal(s *rolloutScope) bool {
	return machineDeploymentIsEqual(r.machineDeployment, s.machineDeployment) && machineSetsAreEqual(r.machineSets, s.machineSets) && machineSetMachinesAreEqual(r.machineSetMachines, s.machineSetMachines)
}

func machineDeploymentIsEqual(a, b *clusterv1.MachineDeployment) bool {
	if upToDate, _, _ := mdutil.MachineTemplateUpToDate(&a.Spec.Template, &b.Spec.Template); !upToDate ||
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
		if upToDate, _, _ := mdutil.MachineTemplateUpToDate(&desiredMS.Spec.Template, &currentMS.Spec.Template); !upToDate ||
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

func maxUnavailableBreachToleration() func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
	return func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
		log.Logf("[Toleration] tolerate maxUnavailable breach")
		return true
	}
}

func maxSurgeToleration() func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
	return func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
		log.Logf("[Toleration] tolerate maxSurge breach")
		return true
	}
}

// default task order ensure the controllers are run in a consistent and predictable way: md, ms1, ms2 and so on.
func defaultTaskOrder(current *rolloutScope) []int {
	taskOrder := []int{}
	for t := range len(current.machineSets) + 1 + 1 { // +1 is for the MachineSet that might be created when reconciling md, +1 is for the md itself
		taskOrder = append(taskOrder, t)
	}
	return taskOrder
}

func randomTaskOrder(current *rolloutScope, rng *rand.Rand) []int {
	u := &UniqueRand{
		rng:       rng,
		generated: map[int]bool{},
		max:       len(current.machineSets) + 1 + 1, // +1 is for the MachineSet that might be created when reconciling md, +1 is for the md itself
	}
	taskOrder := []int{}
	for {
		n := u.Int()
		if rng.Intn(10) < 3 { // skip a step in the 30% of cases
			continue
		}
		taskOrder = append(taskOrder, n)
		if r := rng.Intn(10); r < 3 { // repeat a step in the 30% of cases
			u.Forget(n)
		}
		if u.Done() {
			break
		}
	}
	return taskOrder
}

func createMD(failureDomain string, replicas int32, maxSurge, maxUnavailable int32) *clusterv1.MachineDeployment {
	return &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "md"},
		Spec: clusterv1.MachineDeploymentSpec{
			// Note: using failureDomain as a template field to determine upToDate
			Template: clusterv1.MachineTemplateSpec{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			Replicas: &replicas,
			Rollout: clusterv1.MachineDeploymentRolloutSpec{
				Strategy: clusterv1.MachineDeploymentRolloutStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
						MaxSurge:       ptr.To(intstr.FromInt32(maxSurge)),
						MaxUnavailable: ptr.To(intstr.FromInt32(maxUnavailable)),
					},
				},
			},
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          &replicas,
			AvailableReplicas: &replicas,
		},
	}
}

func createMS(name, failureDomain string, replicas int32) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
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
