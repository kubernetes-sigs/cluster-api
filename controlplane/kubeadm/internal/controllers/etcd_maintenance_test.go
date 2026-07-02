/*
Copyright 2026 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"slices"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

// defragEntries is a test helper that builds a []EtcdMemberDefragStatus from a name→ago map.
func defragEntries(m map[string]time.Duration) []controlplanev1.EtcdMemberDefragStatus {
	if len(m) == 0 {
		return nil
	}
	out := make([]controlplanev1.EtcdMemberDefragStatus, 0, len(m))
	for name, ago := range m {
		out = append(out, controlplanev1.EtcdMemberDefragStatus{
			Name:           name,
			LastDefragTime: metav1.NewTime(time.Now().Add(-ago)),
		})
	}
	return out
}

func TestReconcileEtcdDefragmentation(t *testing.T) {
	ctx := context.Background()

	const (
		// defragRule triggers when the database is more than 80% of the quota.
		defragRule = "dbQuotaUsage > 0.8"

		dbQuota     int64 = 2_000_000_000 // 2 GiB
		dbSizeBig   int64 = 1_800_000_000 // quotaUsage = 0.90 → needs defrag
		dbSizeSmall int64 = 1_000_000_000 // quotaUsage = 0.50 → no defrag
	)

	// memberStatus builds a MemberStatus where leadership is expressed as ID == Leader.
	memberStatus := func(dbSize int64, isLeader bool) *etcd.MemberStatus {
		const leaderID uint64 = 99
		id := uint64(1)
		if isLeader {
			id = leaderID
		}
		return &etcd.MemberStatus{
			ID:          id,
			Leader:      leaderID,
			DbSize:      dbSize,
			DbSizeInUse: dbSize / 2,
			DbSizeQuota: dbQuota,
		}
	}

	tests := []struct {
		name string
		// setup returns the ControlPlane and the fakeWorkloadCluster so the test
		// can inspect DefraggedMembers after the reconcile call.
		setup func() (*internal.ControlPlane, *fakeWorkloadCluster)
		// wantDefragged is the ordered list of members expected to have been defragmented.
		wantDefragged []string
		wantRequeue   bool
		wantErr       bool
		// wantDefragTimesSet is true when EtcdMemberDefragTimes should be populated
		// for each member in wantDefragged.
		wantDefragTimesSet bool
		// wantDisarmed is the ordered list of DisarmEtcdMemberNoSpaceAlarm calls expected
		// (by node name, in call order). A member may appear more than once if both the
		// independent disalarm pass and the post-defrag pass fire in the same reconcile.
		wantDisarmed []string
	}{
		{
			name: "no-op when EtcdMaintenance is nil",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{}
				cp := &internal.ControlPlane{
					KCP:         &controlplanev1.KubeadmControlPlane{},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
		},
		{
			name: "no-op when DefragRule is empty",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: ""},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
		},
		{
			name: "no-op when etcd is external",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
							KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
								ClusterConfiguration: bootstrapv1.ClusterConfiguration{
									Etcd: bootstrapv1.Etcd{
										External: bootstrapv1.ExternalEtcd{
											Endpoints: []string{"https://etcd.example.com:2379"},
										},
									},
								},
							},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
		},
		{
			name: "no-op when EtcdMembers is empty",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster: &clusterv1.Cluster{},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
		},
		{
			// Rule is configured but currently evaluates to false for all members.
			// The controller must still self-schedule a re-evaluation at minInterval
			// rather than relying on SyncPeriod as the sole periodic trigger.
			name: "requeues at minInterval when rule is not satisfied for any member",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeSmall, false),
						"node-b": memberStatus(dbSizeSmall, true),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue: true,
		},
		{
			// Single-member cluster: after defragging the only member, earliestRequeue
			// is 0 (it was computed before the defrag stamp). The controller must still
			// schedule a wakeup at minInterval for the next evaluation cycle.
			name: "requeues at minInterval after defragging the only candidate (single member)",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-a"},
			wantRequeue:        true,
			wantDefragTimesSet: true,
		},
		{
			name: "follower is defragged before leader when both need defrag",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, true),  // leader
						"node-b": memberStatus(dbSizeBig, false), // follower
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-b"},
			wantRequeue:        true,
			wantDefragTimesSet: true,
		},
		{
			name: "requeue when more than one member needs defrag",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, false),
						"node-b": memberStatus(dbSizeBig, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-a"}, // "node-a" < "node-b", both followers
			wantRequeue:        true,
			wantDefragTimesSet: true,
		},
		{
			// Only one of two members needs defrag; no other member is throttled.
			// After the defrag, earliestRequeue is 0 (computed before the stamp), so the
			// controller must schedule a requeue at minInterval for the next evaluation.
			name: "requeues at minInterval after defragging the only member that needed defrag",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, false),
						"node-b": memberStatus(dbSizeSmall, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-a"},
			wantRequeue:        true,
			wantDefragTimesSet: true,
		},
		{
			// The only member is ineligible (empty name), so no defrag runs.
			// Rule is still configured, so the controller must requeue at minInterval.
			name: "requeues at minInterval when only member has empty name",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"": memberStatus(dbSizeBig, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: ""}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue: true,
		},
		{
			// The only member is a learner (ineligible), so no defrag runs.
			// Rule is still configured, so the controller must requeue at minInterval.
			name: "requeues at minInterval when only member is a learner",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a", IsLearner: true}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue: true,
		},
		{
			// node-a errors and is skipped; node-b is defragged as the sole candidate.
			// After the defrag, earliestRequeue is 0 so the controller must requeue at minInterval.
			name: "requeues at minInterval when one member errors and the other is defragged",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatusErrors: map[string]error{
						"node-a": errors.New("connection refused"),
					},
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-b": memberStatus(dbSizeBig, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-b"},
			wantRequeue:        true,
			wantDefragTimesSet: true,
		},
		{
			// A defrag failure must not propagate as a reconciler error (which would
			// trigger the per-object backoff rate-limiter and delay scaling / remediation).
			// Instead the controller must log the error and requeue for a prompt retry
			// without stamping the defrag timestamp (so the member stays eligible).
			name: "DefragEtcdMember error is logged and retried via RequeueAfter",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeBig, false),
					},
					DefragEtcdMemberErr: errors.New("defrag failed"),
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
						},
					},
					Cluster:     &clusterv1.Cluster{},
					EtcdMembers: []*etcd.Member{{Name: "node-a"}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			// DefragEtcdMember is called (attempt recorded) but no timestamp stamped.
			wantDefragged: []string{"node-a"},
			wantRequeue:   true,
			// wantDefragTimesSet intentionally false: member must remain eligible for retry.
		},
		{
			// AutoDisalarm=false: even with an active NOSPACE alarm and db ratio below the
			// threshold, no disarm should be attempted because the feature is disabled.
			name: "auto-disalarm disabled: no disarm even when member has NOSPACE alarm below threshold",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeSmall, false), // ratio 0.50, below default threshold
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
								DefragRule:   defragRule, // ratio 0.50 < 0.80 → no defrag
								AutoDisalarm: false,
							},
						},
					},
					Cluster:           &clusterv1.Cluster{},
					EtcdMembers:       []*etcd.Member{{Name: "node-a", ID: 1}},
					EtcdMembersAlarms: []etcd.MemberAlarm{{MemberID: 1, Type: etcd.AlarmNoSpace}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue: true,
			// wantDisarmed intentionally nil: AutoDisalarm=false must suppress all disarm calls.
		},
		{
			// AutoDisalarm=true, independent pass: member has a NOSPACE alarm and its db ratio
			// (0.50) is below the default 90% threshold → alarm is cleared before defrag.
			name: "auto-disalarm: independent pass disarms member with NOSPACE alarm below threshold",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeSmall, false), // ratio 0.50 < 0.90 threshold
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
								DefragRule:   defragRule, // ratio 0.50 < 0.80 → no defrag
								AutoDisalarm: true,
							},
						},
					},
					Cluster:           &clusterv1.Cluster{},
					EtcdMembers:       []*etcd.Member{{Name: "node-a", ID: 1}},
					EtcdMembersAlarms: []etcd.MemberAlarm{{MemberID: 1, Type: etcd.AlarmNoSpace}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue:  true,
			wantDisarmed: []string{"node-a"},
		},
		{
			// AutoDisalarm=true but the db ratio (0.50) is above a custom 40% threshold, so
			// the alarm must not be cleared.
			name: "auto-disalarm: independent pass skips disarm when ratio is above custom threshold",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeSmall, false), // ratio 0.50 >= 0.40 threshold
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
								DefragRule:               defragRule, // ratio 0.50 < 0.80 → no defrag
								AutoDisalarm:             true,
								DisalarmThresholdPercent: 40,
							},
						},
					},
					Cluster:           &clusterv1.Cluster{},
					EtcdMembers:       []*etcd.Member{{Name: "node-a", ID: 1}},
					EtcdMembersAlarms: []etcd.MemberAlarm{{MemberID: 1, Type: etcd.AlarmNoSpace}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue: true,
			// wantDisarmed intentionally nil: ratio 0.50 >= threshold 0.40 → no disarm.
		},
		{
			// AutoDisalarm=true, defrag rule always true, ratio below 90% threshold.
			// The independent pass disarms the member before defrag; then, after the successful
			// defrag, the post-defrag pass disarms it again — so DisarmEtcdMemberNoSpaceAlarm
			// is called twice in the same reconcile.
			name: "auto-disalarm: disarmed by independent pass and again immediately after successful defrag",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						// ratio 0.50 < 0.90 threshold → disarm; "dbSize >= 0" → always defrag
						"node-a": memberStatus(dbSizeSmall, false),
					},
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
								DefragRule:   "dbSize >= 0",
								AutoDisalarm: true,
							},
						},
					},
					Cluster:           &clusterv1.Cluster{},
					EtcdMembers:       []*etcd.Member{{Name: "node-a", ID: 1}},
					EtcdMembersAlarms: []etcd.MemberAlarm{{MemberID: 1, Type: etcd.AlarmNoSpace}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantDefragged:      []string{"node-a"},
			wantRequeue:        true,
			wantDefragTimesSet: true,
			// Disarm fires once in the independent pass (before defrag) and once in the
			// post-defrag pass — both see ratio 0.50 < 0.90.
			wantDisarmed: []string{"node-a", "node-a"},
		},
		{
			// A DisarmEtcdMemberNoSpaceAlarm error must be absorbed: the reconciler must not
			// return an error, and all other processing must continue unaffected.
			name: "auto-disalarm: disarm error is swallowed and reconcile succeeds",
			setup: func() (*internal.ControlPlane, *fakeWorkloadCluster) {
				w := &fakeWorkloadCluster{
					EtcdMemberStatuses: map[string]*etcd.MemberStatus{
						"node-a": memberStatus(dbSizeSmall, false), // ratio 0.50 < 0.90 → disarm attempted
					},
					DisarmEtcdMemberNoSpaceAlarmErr: errors.New("disarm RPC failed"),
				}
				cp := &internal.ControlPlane{
					KCP: &controlplanev1.KubeadmControlPlane{
						Spec: controlplanev1.KubeadmControlPlaneSpec{
							EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
								DefragRule:   defragRule, // ratio 0.50 < 0.80 → no defrag
								AutoDisalarm: true,
							},
						},
					},
					Cluster:           &clusterv1.Cluster{},
					EtcdMembers:       []*etcd.Member{{Name: "node-a", ID: 1}},
					EtcdMembersAlarms: []etcd.MemberAlarm{{MemberID: 1, Type: etcd.AlarmNoSpace}},
				}
				cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})
				return cp, w
			},
			wantRequeue:  true,
			wantDisarmed: []string{"node-a"}, // call was made despite the error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cp, workload := tt.setup()
			r := &KubeadmControlPlaneReconciler{}
			result, err := r.reconcileEtcdDefragmentation(ctx, cp)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			if tt.wantRequeue {
				g.Expect(result.RequeueAfter).To(BeNumerically(">", time.Duration(0)))
			} else {
				g.Expect(result.RequeueAfter).To(BeZero())
			}

			g.Expect(workload.DefraggedMembers).To(Equal(tt.wantDefragged))
			g.Expect(workload.DisarmedMembers).To(Equal(tt.wantDisarmed))

			if tt.wantDefragTimesSet {
				g.Expect(cp.KCP.Status.EtcdMemberDefragTimes).ToNot(BeEmpty(),
					"EtcdMemberDefragTimes should be populated when a defrag occurs")
				for _, name := range tt.wantDefragged {
					ts := memberDefragTime(cp.KCP.Status.EtcdMemberDefragTimes, name)
					g.Expect(ts).ToNot(BeNil(),
						"EtcdMemberDefragTimes should contain an entry for member %q", name)
					g.Expect(ts.IsZero()).To(BeFalse(),
						"defrag timestamp for member %q should not be zero", name)
				}
			} else {
				g.Expect(cp.KCP.Status.EtcdMemberDefragTimes).To(BeNil(),
					"EtcdMemberDefragTimes should remain nil when no defrag occurred")
			}
		})
	}
}

func TestReconcileEtcdDefragmentation_MinDefragInterval(t *testing.T) {
	ctx := context.Background()
	const defragRule = "dbSize >= 0" // always true

	baseStatus := func() *etcd.MemberStatus {
		return &etcd.MemberStatus{ID: 1, Leader: 99, DbSize: 1_000, DbSizeInUse: 800, DbSizeQuota: 2_000_000_000}
	}

	tests := []struct {
		name string
		// minDefragIntervalSeconds overrides the default when non-zero.
		minDefragIntervalSeconds int32
		// initialDefragTimes seeds EtcdMemberDefragTimes before the reconcile (name → how long ago).
		initialDefragAgo map[string]time.Duration
		// wantDefragged lists the members that should have been defragmented.
		wantDefragged []string
		// wantRequeueMin/Max bound the expected RequeueAfter (both zero means no requeue expected).
		wantRequeueMin time.Duration
		wantRequeueMax time.Duration
		// wantMemberDefragUpdated lists members whose entry should be freshly stamped.
		wantMemberDefragUpdated []string
	}{
		{
			// First-ever defrag: no previous throttle, so earliestRequeue is 0 after
			// defragging. The controller must self-schedule via minInterval (1h default).
			name:                    "no EtcdMemberDefragTimes entry: defrag runs (first run)",
			wantDefragged:           []string{"node-a"},
			wantMemberDefragUpdated: []string{"node-a"},
			wantRequeueMin:          55 * time.Minute,
			wantRequeueMax:          65 * time.Minute,
		},
		{
			// Interval elapsed (2h > 1h): defrag runs again. Same as first-run: no other
			// throttled member exists, so the controller schedules via minInterval (1h).
			name:                    "last defrag for member exceeded default 1h interval: defrag runs",
			initialDefragAgo:        map[string]time.Duration{"node-a": 2 * time.Hour},
			wantDefragged:           []string{"node-a"},
			wantMemberDefragUpdated: []string{"node-a"},
			wantRequeueMin:          55 * time.Minute,
			wantRequeueMax:          65 * time.Minute,
		},
		{
			name:             "last defrag for member within default 1h interval: skipped, requeues for remainder",
			initialDefragAgo: map[string]time.Duration{"node-a": 30 * time.Minute},
			wantRequeueMin:   25 * time.Minute,
			wantRequeueMax:   30*time.Minute + 5*time.Second,
		},
		{
			name:                     "custom MinDefragIntervalSeconds not yet elapsed: skipped, requeues for remainder",
			minDefragIntervalSeconds: 600, // 10 minutes
			initialDefragAgo:         map[string]time.Duration{"node-a": 3 * time.Minute},
			wantRequeueMin:           6 * time.Minute,
			wantRequeueMax:           7*time.Minute + 5*time.Second,
		},
		{
			// Custom interval (10m) elapsed: defrag runs. Afterwards the controller must
			// schedule a requeue at the custom minInterval (10m) for the next evaluation.
			name:                     "custom MinDefragIntervalSeconds elapsed: defrag runs",
			minDefragIntervalSeconds: 600, // 10 minutes
			initialDefragAgo:         map[string]time.Duration{"node-a": 15 * time.Minute},
			wantDefragged:            []string{"node-a"},
			wantMemberDefragUpdated:  []string{"node-a"},
			wantRequeueMin:           9 * time.Minute,
			wantRequeueMax:           11 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			initial := defragEntries(tt.initialDefragAgo)

			w := &fakeWorkloadCluster{
				EtcdMemberStatuses: map[string]*etcd.MemberStatus{"node-a": baseStatus()},
			}
			cp := &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{
							DefragRule:               defragRule,
							MinDefragIntervalSeconds: tt.minDefragIntervalSeconds,
						},
					},
					Status: controlplanev1.KubeadmControlPlaneStatus{
						EtcdMemberDefragTimes: initial,
					},
				},
				Cluster:     &clusterv1.Cluster{},
				EtcdMembers: []*etcd.Member{{Name: "node-a"}},
			}
			cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})

			r := &KubeadmControlPlaneReconciler{}
			result, err := r.reconcileEtcdDefragmentation(ctx, cp)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(w.DefraggedMembers).To(Equal(tt.wantDefragged))

			if tt.wantRequeueMin > 0 {
				g.Expect(result.RequeueAfter).To(BeNumerically(">", tt.wantRequeueMin))
				g.Expect(result.RequeueAfter).To(BeNumerically("<=", tt.wantRequeueMax))
			} else {
				g.Expect(result.RequeueAfter).To(BeZero())
			}

			for _, name := range tt.wantMemberDefragUpdated {
				ts := memberDefragTime(cp.KCP.Status.EtcdMemberDefragTimes, name)
				g.Expect(ts).ToNot(BeNil(), "EtcdMemberDefragTimes should have an entry for %q", name)
				g.Expect(ts.IsZero()).To(BeFalse(), "defrag timestamp for %q should not be zero", name)
				if prevAgo, had := tt.initialDefragAgo[name]; had {
					prevTime := time.Now().Add(-prevAgo)
					g.Expect(ts.Time).To(BeTemporally(">", prevTime),
						"defrag timestamp for %q should be updated past the initial value", name)
				}
			}

			// Members not in wantMemberDefragUpdated should be unchanged from initialDefragAgo.
			for name, ago := range tt.initialDefragAgo {
				if slices.Contains(tt.wantMemberDefragUpdated, name) {
					continue
				}
				ts := memberDefragTime(cp.KCP.Status.EtcdMemberDefragTimes, name)
				g.Expect(ts).ToNot(BeNil())
				// The stored time should still be approximately (now - ago), not refreshed.
				expectedTime := time.Now().Add(-ago)
				g.Expect(ts.Time).To(BeTemporally("~", expectedTime, 5*time.Second),
					"EtcdMemberDefragTimes[%q] should not have been refreshed", name)
			}
		})
	}
}

// TestReconcileEtcdDefragmentation_PerMemberIndependence verifies that per-member interval
// enforcement is truly independent: a member whose interval has not elapsed is skipped while
// a member whose interval has elapsed IS defragmented in the same reconcile pass.
func TestReconcileEtcdDefragmentation_PerMemberIndependence(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	const defragRule = "dbSize >= 0" // always true

	baseStatus := func() *etcd.MemberStatus {
		return &etcd.MemberStatus{ID: 1, Leader: 99, DbSize: 1_000, DbSizeInUse: 800, DbSizeQuota: 2_000_000_000}
	}

	w := &fakeWorkloadCluster{
		EtcdMemberStatuses: map[string]*etcd.MemberStatus{
			"node-a": baseStatus(),
			"node-b": baseStatus(),
		},
	}

	// node-a was defragged 2 hours ago (interval elapsed); node-b was defragged 5 minutes ago (still throttled).
	cp := &internal.ControlPlane{
		KCP: &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				EtcdMaintenance: &controlplanev1.EtcdMaintenanceSpec{DefragRule: defragRule},
			},
			Status: controlplanev1.KubeadmControlPlaneStatus{
				EtcdMemberDefragTimes: []controlplanev1.EtcdMemberDefragStatus{
					{Name: "node-a", LastDefragTime: metav1.NewTime(time.Now().Add(-2 * time.Hour))},
					{Name: "node-b", LastDefragTime: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
				},
			},
		},
		Cluster:     &clusterv1.Cluster{},
		EtcdMembers: []*etcd.Member{{Name: "node-a"}, {Name: "node-b"}},
	}
	cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: w, Reader: fake.NewFakeClient()})

	r := &KubeadmControlPlaneReconciler{}
	result, err := r.reconcileEtcdDefragmentation(ctx, cp)

	g.Expect(err).ToNot(HaveOccurred())
	// Only node-a should be defragged; node-b is still within its interval.
	g.Expect(w.DefraggedMembers).To(Equal([]string{"node-a"}))
	// The controller should requeue when node-b becomes eligible (~55 minutes from now).
	g.Expect(result.RequeueAfter).To(BeNumerically(">", 50*time.Minute))
	g.Expect(result.RequeueAfter).To(BeNumerically("<=", 55*time.Minute+5*time.Second))
	// node-a's timestamp should be updated; node-b's should be unchanged (~5 minutes ago).
	tsA := memberDefragTime(cp.KCP.Status.EtcdMemberDefragTimes, "node-a")
	g.Expect(tsA.Time).To(BeTemporally(">", time.Now().Add(-10*time.Second)))
	tsB := memberDefragTime(cp.KCP.Status.EtcdMemberDefragTimes, "node-b")
	g.Expect(tsB.Time).To(BeTemporally("~", time.Now().Add(-5*time.Minute), 5*time.Second))
}
