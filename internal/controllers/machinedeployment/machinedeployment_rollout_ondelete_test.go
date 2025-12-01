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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

func TestReconcileOldMachineSetsOnDelete(t *testing.T) {
	testCases := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		scaleIntent       map[string]int32
		newMachineSet     *clusterv1.MachineSet
		oldMachineSets    []*clusterv1.MachineSet
		expectScaleIntent map[string]int32
		expectedNotes     map[string][]string
		error             error
	}{
		{
			name: "OnDelete strategy: do not scale oldMSs when no machines have been deleted",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "md",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
						},
					},
					Replicas: ptr.To[int32](4),
				},
			},
			scaleIntent: map[string]int32{},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms3",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms1",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](3),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](3),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms2",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](1),
					},
				},
			},
			expectScaleIntent: map[string]int32{
				// no new intent for old MS
			},
			expectedNotes: map[string][]string{},
		},
		{
			name: "OnDelete strategy: scale down oldMSs when machines have been deleted",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "md",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
						},
					},
					Replicas: ptr.To[int32](3),
				},
			},
			scaleIntent: map[string]int32{},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms3",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms1",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](3),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](2), // one machines has been deleted
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms2",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](0), // one machines has been deleted
					},
				},
			},
			expectScaleIntent: map[string]int32{
				// new intent for old MS
				"ms1": 2, // scale down by one, one machines has been deleted
				"ms2": 0, // scale down by one, one machines has been deleted
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align to existing Machines"},
				"ms2": {"scale down to align to existing Machines"},
			},
		},
		{
			name: "OnDelete strategy: scale down oldMSs when md has been scaled down",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "md",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
						},
					},
					Replicas: ptr.To[int32](3), // scaled down to 3 replicas, there are exceeding machines in the cluster
				},
			},
			scaleIntent: map[string]int32{},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms3",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms1",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](1),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms2",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](2),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](2),
					},
				},
			},
			expectScaleIntent: map[string]int32{
				// new intent for old MS
				"ms1": 0, // scale down by one, 1 exceeding machine removed from ms1
				"ms2": 1, // scale down by one, 1 exceeding machine removed from ms2
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
				"ms2": {"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
			},
		},
		{
			name: "OnDelete strategy: scale down oldMSs when md has been scaled down, keeps into account newMS scale intent",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "md",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
						},
					},
					Replicas: ptr.To[int32](3), // scaled down to 3 replicas, there are exceeding machines in the cluster
				},
			},
			scaleIntent: map[string]int32{
				"ms3": 2, // scaling intent for new MS (+1)
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms3",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](1),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms1",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](1),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms2",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](2),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](2),
					},
				},
			},
			expectScaleIntent: map[string]int32{
				"ms3": 2,
				// new intent for old MS
				"ms1": 0, // scale down by one, 1 exceeding machine removed from ms1
				"ms2": 1, // scale down by one, 1 exceeding machine removed from ms2
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
				"ms2": {"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
			},
		},
		{
			name: "OnDelete strategy: scale down oldMSs when md has been scaled down keeps into account deleted machines",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "md",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
						},
					},
					Replicas: ptr.To[int32](3), // scaled down to 3 replicas, there are exceeding machines in the cluster
				},
			},
			scaleIntent: map[string]int32{},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ms3",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms1",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](0), // one machines has been deleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ms2",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](2),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](2),
					},
				},
			},
			expectScaleIntent: map[string]int32{
				// new intent for old MS
				"ms1": 0, // scale down by one, one machines has been deleted
				"ms2": 1, // scale down by one, 1 exceeding machine removed from ms1
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align to existing Machines"},
				"ms2": {"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			planner := newRolloutPlanner(nil, nil, nil)
			planner.scaleIntents = tt.scaleIntent
			planner.md = tt.machineDeployment
			planner.newMS = tt.newMachineSet
			planner.oldMSs = tt.oldMachineSets

			planner.reconcileOldMachineSetsOnDelete(ctx)
			g.Expect(planner.scaleIntents).To(Equal(tt.expectScaleIntent), "unexpected scaleIntents")
			g.Expect(planner.notes).To(Equal(tt.expectedNotes), "unexpected notes")
		})
	}
}

type onDeleteSequenceTestCase struct {
	name string

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

	// desiredMachineNames is the list of machines at the end of the rollout.
	// all the machines in this list are expected to be upToDate and owned by the new MS after the rollout (which is different from the new MS before the rollout).
	// if this list contains old machines names (machine names already in currentMachineNames), it implies those machine have been upgraded in places.
	// if this list contains new machines names (machine names not in currentMachineNames), it implies those machines have been created during a rollout;
	// please name new machines names as "mX" where X is a progressive number starting after the max number in currentMachineNames (do not skip numbers),
	// e.g. desiredMachineNames "m4","m5","m6" (desired machine names after a regular rollout of a MD with currentMachineNames "m1","m2","m3")
	// e.g. desiredMachineNames "m1","m2","m3" (desired machine names after rollout performed using in-place update for an MD with currentMachineNames "m1","m2","m3")
	desiredMachineNames []string

	// maxUserUnavailable define the maximum numbers of unavailable machines a user want to have in the system.
	// Unavailable machines includes both machines being deleted or machines already deleted.
	maxUserUnavailable int32

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

func Test_OnDeleteSequences(t *testing.T) {
	tests := []onDeleteSequenceTestCase{
		{ // delete 1
			name:                "3 replicas, maxUserUnavailable 1",
			currentMachineNames: []string{"m1", "m2", "m3"},
			maxUserUnavailable:  1,
			desiredMachineNames: []string{"m4", "m5", "m6"},
		},
		{ // delete 2
			name:                "6 replicas, maxUserUnavailable 2",
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			maxUserUnavailable:  2,
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // delete 1 + scale up machine deployment in the middle
			name: "6 Replicas, maxUserUnavailable 1, scale up to 9",
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD scaled up to 9.
				machineDeployment: createMD("v2", 9, withOnDeleteStrategy()),
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
			maxUserUnavailable:  1,
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12", "m13", "m14", "m15"},
		},
		{ // delete 1 + scale down machine deployment in the middle
			name: "12 Replicas, maxUserUnavailable 1,  scale down to 6",
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 12 replica in the middle of a rollout, with 3 machines already created in the newMS and 9 still on the oldMS, and then MD scaled down to 6.
				machineDeployment: createMD("v2", 6, withOnDeleteStrategy()),
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
			maxUserUnavailable:  1,
			desiredMachineNames: []string{"m13", "m14", "m15", "m16", "m17", "m18"},
		},
		{ // delete 1 + change spec in the middle
			name: "6 Replicas, maxUserUnavailable 1, change spec",
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD spec is changed.
				machineDeployment: createMD("v3", 6, withOnDeleteStrategy()),
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
			maxUserUnavailable:  1,
			desiredMachineNames: []string{"m10", "m11", "m12", "m13", "m14", "m15"}, // NOTE: Machines created before the spec change are deleted
		},
	}

	testWithPredictableReconcileOrder := true
	testWithRandomReconcileOrderFromConstantSeed := true
	testWithRandomReconcileOrderFromRandomSeed := true

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.name

			if testWithPredictableReconcileOrder {
				tt.maxIterations = 50
				tt.randomControllerOrder = false
				tt.logAndGoldenFileName = strings.ToLower(tt.name)
				t.Run("default", func(t *testing.T) {
					runOnDeleteTestCase(ctx, t, tt)
				})
			}

			if testWithRandomReconcileOrderFromConstantSeed {
				tt.maxIterations = 70
				tt.name = fmt.Sprintf("%s, random(0)", name)
				tt.randomControllerOrder = true
				tt.seed = 0
				tt.logAndGoldenFileName = strings.ToLower(tt.name)
				t.Run("random(0)", func(t *testing.T) {
					runOnDeleteTestCase(ctx, t, tt)
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
						runOnDeleteTestCase(ctx, t, tt)
					})
				}
			}
		})
	}
}

func runOnDeleteTestCase(ctx context.Context, t *testing.T, tt onDeleteSequenceTestCase) {
	t.Helper()
	g := NewWithT(t)

	rng := rand.New(rand.NewSource(tt.seed)) //nolint:gosec // it is ok to use a weak randomizer here
	fLogger := newFileLogger(t, tt.name, fmt.Sprintf("testdata/ondelete/%s", tt.logAndGoldenFileName))
	// uncomment this line to automatically generate/update golden files: fLogger.writeGoldenFile = true

	// Init current and desired state from test case
	current := tt.currentScope.Clone()
	if current == nil {
		current = initCurrentRolloutScope(tt.currentMachineNames, withOnDeleteStrategy())
	}
	desired := computeDesiredRolloutScope(current, tt.desiredMachineNames)

	// Log initial state
	fLogger.Logf("[Test] Initial state\n%s", current.summary())
	random := ""
	if tt.randomControllerOrder {
		random = fmt.Sprintf(", random(%d)", tt.seed)
	}
	fLogger.Logf("[Test] Rollout %d replicas, onDeleteStrategy%s\n", len(current.machines()), random)
	i := 1
	maxIterations := tt.maxIterations

	// Prevent deletion to start till the rollout planner did create the newMS.
	// Note: this prevent machines being deleted and re-created on oldMSs in case of random sequences.
	canDelete := false

	for {
		taskList := getTaskListOnDelete(current)
		taskCount := len(taskList)
		taskOrder := defaultTaskOrder(taskCount)
		if tt.randomControllerOrder {
			taskOrder = randomTaskOrder(taskCount, rng)
		}
		for _, taskID := range taskOrder {
			task := taskList[taskID]
			if task == "md" {
				fLogger.Logf("[MD controller] Iteration %d, Reconcile md", i)

				// Running a small subset of MD reconcile (the rollout logic and a bit of setReplicas)
				p := newRolloutPlanner(nil, nil, nil)
				p.overrideComputeDesiredMS = func(ctx context.Context, deployment *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
					log := ctrl.LoggerFrom(ctx)
					desiredNewMS := currentNewMS
					if currentNewMS == nil {
						// uses a predictable MS name when creating newMS, also add the newMS to current.machineSets
						totMS := len(current.machineSets)
						desiredNewMS = createMS(fmt.Sprintf("ms%d", totMS+1), deployment.Spec.Template.Spec.FailureDomain, 0)
						current.machineSets = append(current.machineSets, desiredNewMS)
						log.V(5).Info(fmt.Sprintf("Computing new MachineSet %s with %d replicas", desiredNewMS.Name, ptr.Deref(desiredNewMS.Spec.Replicas, 0)), "MachineSet", klog.KObj(desiredNewMS))
					}
					return desiredNewMS, nil
				}

				// init the rollout planner and plan next step for a rollout.
				err := p.init(ctx, current.machineDeployment, current.machineSets, current.machines(), true, true)
				g.Expect(err).ToNot(HaveOccurred())

				err = p.planOnDelete(ctx)
				g.Expect(err).ToNot(HaveOccurred())

				// Apply changes.
				for _, ms := range current.machineSets {
					if scaleIntent, ok := p.scaleIntents[ms.Name]; ok {
						ms.Spec.Replicas = ptr.To(scaleIntent)
					}
				}

				// As soon as the newMS exists, unblock machine deletion (at this stage we are sure new machines will be created on the newMS only).
				if p.newMS != nil {
					canDelete = true
				}

				// Running a small subset of setReplicas (we don't want to run the full func to avoid unnecessary noise on the test)
				current.machineDeployment.Status.Replicas = mdutil.GetActualReplicaCountForMachineSets(current.machineSets)
				current.machineDeployment.Status.AvailableReplicas = mdutil.GetAvailableReplicaCountForMachineSets(current.machineSets)

				// Log state after this reconcile
				fLogger.Logf("[MD controller] - Result of rollout planner\n%s", current.rolloutPlannerResultSummary(p))
			}

			// Simulate the user deleting machines not upToDate; in order to make this realistic deletion will be performed
			// if the operation respects maxUserUnavailable e.g.
			// - with maxUserUnavailable=1, perform deletion only when previous deletion has been completed and a replacement machines
			//   has been created, which we assume it is the most common behaviour.
			// - maxUserUnavailable > 1 can be used to test scenarios where the users delete machines in a more aggressive way.
			// Also, deletion cannot happen before the newMS has been created (thus preventing machines being deleted and
			// re-created on oldMSs in case of random sequences, which will lead to result different from what expected)
			if task == "delete-machine" {
				deletionsInFlight := int32(0)
				for _, m := range current.machines() {
					if !m.DeletionTimestamp.IsZero() {
						deletionsInFlight++
					}
				}

				totAvailable := max(int32(len(current.machines()))-deletionsInFlight, 0)
				minAvailable := max(ptr.Deref(current.machineDeployment.Spec.Replicas, 0)-tt.maxUserUnavailable, 0)
				if totAvailable > minAvailable && canDelete {
					// Determine the list of machines that should be deleted manually, which are the machines not yet UpToDate.
					machinesToDelete := []*clusterv1.Machine{}
					for _, m := range current.machines() {
						if upToDate, _ := mdutil.MachineTemplateUpToDate(&clusterv1.MachineTemplateSpec{Spec: m.Spec}, &desired.machineDeployment.Spec.Template); !upToDate {
							machinesToDelete = append(machinesToDelete, m)
						}
					}

					if len(machinesToDelete) > 0 {
						// When running with default controller order, also delete machines in order;
						// conversely, when running with random controller order, also machine deletion happens in random order.
						n := 0
						if tt.randomControllerOrder {
							n = rng.Intn(len(machinesToDelete))
						}

						if machinesToDelete[n].DeletionTimestamp.IsZero() {
							fLogger.Logf("[User] Iteration %d, Deleting machine %s", i, machinesToDelete[n].Name)
							machinesToDelete[n].DeletionTimestamp = ptr.To(metav1.Now())
						}
					}
				}
			}

			// Run mutators faking MS controllers
			for _, ms := range current.machineSets {
				if ms.Name == task {
					fLogger.Logf("[MS controller] Iteration %d, Reconcile %s", i, current.machineSetSummary(ms))
					err := machineSetControllerMutator(fLogger, ms, current)
					g.Expect(err).ToNot(HaveOccurred())
					break
				}
			}

			// Run mutators faking M controllers
			for _, ms := range current.machineSets {
				for _, m := range current.machineSetMachines[ms.Name] {
					if m.Name == task {
						fLogger.Logf("[M controller] Iteration %d, Reconcile %s", i, m.Name)
						machineControllerMutator(fLogger, m, current)
					}
				}
			}
		}

		// Check if we are at the desired state
		if current.Equal(desired) {
			fLogger.Logf("[Test] Final state\n%s", current.summary())
			break
		}

		// Safeguard for infinite reconcile
		i++
		if i > maxIterations {
			// NOTE: the following can be used to set a breakpoint for debugging why the system is not reaching desired state after maxIterations (to check what is not yet equal)
			current.Equal(desired)
			// Log desired state we never reached
			fLogger.Logf("[Test] Desired state\n%s", desired.summary())
			g.Fail(fmt.Sprintf("Failed to reach desired state in %d iterations", maxIterations))
		}
	}

	if !tt.skipLogToFileAndGoldenFileCheck {
		currentLog, goldenLog, err := fLogger.WriteLogAndCompareWithGoldenFile()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentLog).To(Equal(goldenLog), "current test case log and golden test case log are different\n%s", cmp.Diff(currentLog, goldenLog))
	}
}

func getTaskListOnDelete(current *rolloutScope) []string {
	taskList := make([]string, 0)
	taskList = append(taskList, "md")
	for _, ms := range current.machineSets {
		taskList = append(taskList, ms.Name)
	}
	taskList = append(taskList,
		fmt.Sprintf("ms%d", len(current.machineSets)+1), // r the MachineSet that might be created when reconciling md
		"delete-machine",
	)
	for _, m := range current.machines() {
		taskList = append(taskList, m.Name)
	}
	return taskList
}
