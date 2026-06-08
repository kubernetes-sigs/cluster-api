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
	"sort"
	"strconv"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

const (
	defaultMinDefragIntervalSeconds int32 = 3600 // 1 hour
	defaultDisalarmThresholdPercent int32 = 90

	// defragErrorRetryInterval is the RequeueAfter used when a DefragEtcdMember call
	// fails.  It is intentionally short so that transient failures are retried promptly
	// without blocking other KCP operations via the per-object error rate-limiter.
	defragErrorRetryInterval = 1 * time.Minute
)

// reconcileEtcdDefragmentation evaluates the configured defrag rule for each managed etcd
// member and, when the rule is satisfied and the per-member minimum interval has elapsed,
// defragments one member per reconcile cycle.
//
// Members are processed in a safe order: followers first, the leader last. Defragmenting
// only one member per reconcile and then requeuing ensures that the cluster is never
// exposed to more than one unavailable member at a time.
//
// The minimum interval (minDefragIntervalSeconds) is enforced independently per member:
// all members in a cluster can be defragmented in rapid succession and each member will
// not be defragmented again until its own timer has expired.
func (r *KubeadmControlPlaneReconciler) reconcileEtcdDefragmentation(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// No-op when defrag is not configured or etcd is not managed by KCP.
	if controlPlane.KCP.Spec.EtcdMaintenance == nil ||
		controlPlane.KCP.Spec.EtcdMaintenance.DefragRule == "" ||
		!controlPlane.IsEtcdManaged() {
		log.Info("etcd auto defragmentation is disabled or etcd is not managed by KCP, skipping")
		return ctrl.Result{}, nil
	}

	// No-op when the etcd member list has not been populated yet.
	if len(controlPlane.EtcdMembers) == 0 {
		return ctrl.Result{}, nil
	}

	// A zero value means "use the default".
	minIntervalSeconds := controlPlane.KCP.Spec.EtcdMaintenance.MinDefragIntervalSeconds
	if minIntervalSeconds == 0 {
		minIntervalSeconds = defaultMinDefragIntervalSeconds
	}
	minInterval := time.Duration(minIntervalSeconds) * time.Second

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to get workload cluster client for etcd defragmentation")
	}

	spec := controlPlane.KCP.Spec.EtcdMaintenance
	etcdLocal := &controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local

	// Pre-compute disalarm threshold once; used by both the independent disalarm pass
	// below and the post-defrag disalarm check further down.
	thresholdPercent := spec.DisalarmThresholdPercent
	if thresholdPercent == 0 {
		thresholdPercent = defaultDisalarmThresholdPercent
	}
	disalarmThresholdRatio := float64(thresholdPercent) / 100.0

	// Independent auto-disalarm pass: runs every reconcile cycle, before the defrag pass.
	// By decoupling disalarm from defrag success we handle the case where a previous defrag
	// succeeded on the etcd server but returned a client-side error — the alarm is cleared
	// on the next reconcile regardless of defrag outcome.
	if spec.AutoDisalarm {
		for _, member := range controlPlane.EtcdMembers {
			if member.Name == "" || member.IsLearner {
				continue
			}
			if !memberHasNoSpaceAlarm(controlPlane.EtcdMembersAlarms, member.ID) {
				continue
			}
			maybeDisarmNoSpaceAlarm(ctx, workloadCluster, member.Name, member.ID, disalarmThresholdRatio, etcdLocal)
		}
	}

	rule := spec.DefragRule

	// Collect members whose defrag rule evaluates to true AND whose per-member interval
	// has elapsed, together with their status so we can sort them without a second round
	// of Status() calls.
	type candidate struct {
		member *etcd.Member
		status *etcd.MemberStatus
	}
	var candidates []candidate
	// earliestRequeue tracks the soonest a currently-throttled member will become eligible,
	// so the controller can requeue exactly when the next member is ready.
	var earliestRequeue time.Duration

	for _, member := range controlPlane.EtcdMembers {
		// Skip members that are not yet named or are learners; they cannot be defragmented safely.
		if member.Name == "" || member.IsLearner {
			continue
		}

		// Enforce the per-member minimum interval.
		if last := memberDefragTime(controlPlane.KCP.Status.EtcdMemberDefragTimes, member.Name); last != nil {
			elapsed := time.Since(last.Time)
			if elapsed < minInterval {
				remaining := minInterval - elapsed
				log.V(4).Info("Skipping etcd defragmentation for member: minimum interval not elapsed",
					"member", member.Name,
					"elapsed", elapsed.Round(time.Second),
					"minInterval", minInterval,
					"requeueAfter", remaining.Round(time.Second))
				if earliestRequeue == 0 || remaining < earliestRequeue {
					earliestRequeue = remaining
				}
				continue
			}
		}

		status, err := workloadCluster.EtcdMemberStatus(ctx, member.Name)
		if err != nil {
			log.Error(err, "Failed to get etcd member status, skipping defragmentation check for member", "member", member.Name)
			continue
		}

		quota := resolveEtcdQuota(status.DbSizeQuota, etcdLocal)
		needsDefrag, err := etcd.EvaluateDefragRule(
			rule,
			status.DbSize,
			status.DbSizeInUse,
			quota,
		)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to evaluate defrag rule for etcd member %s", member.Name)
		}

		if needsDefrag {
			candidates = append(candidates, candidate{member: member, status: status})
		}
	}

	if len(candidates) == 0 {
		log.Info("No etcd member need to be defragmented")
		// No member is ready to defrag right now. Requeue when the earliest throttled
		// member becomes eligible (if any).
		if earliestRequeue > 0 {
			return ctrl.Result{RequeueAfter: earliestRequeue}, nil
		}
		// The rule is configured but no defrag is needed right now and no throttle is
		// active. Schedule a recheck at minInterval so the controller is self-sufficient
		// and does not rely solely on the manager's SyncPeriod for the next evaluation.
		return ctrl.Result{RequeueAfter: minInterval}, nil
	}

	// Sort: followers before the leader to minimise disruption. Ties are broken by member
	// name for determinism.
	sort.Slice(candidates, func(i, j int) bool {
		iIsLeader := candidates[i].status.ID == candidates[i].status.Leader
		jIsLeader := candidates[j].status.ID == candidates[j].status.Leader
		if iIsLeader != jIsLeader {
			// non-leader (false) sorts before leader (true)
			return !iIsLeader
		}
		return candidates[i].member.Name < candidates[j].member.Name
	})

	// Defragment one member per reconcile to avoid simultaneous unavailability.
	target := candidates[0]
	log.Info("Defragmenting etcd member", "candidates", len(candidates), "member", target.member.Name,
		"dbSize", target.status.DbSize, "dbSizeInUse", target.status.DbSizeInUse)

	if err := workloadCluster.DefragEtcdMember(ctx, target.member.Name); err != nil {
		// Treat defrag failures as a soft error: log and requeue for a prompt retry
		// rather than returning an error that would trigger the per-object exponential
		// backoff rate-limiter and delay all other KCP operations (scaling, rolling
		// updates, remediation) for this cluster.  The defrag timestamp is intentionally
		// NOT stamped so that the member remains eligible on the next attempt.
		// Note: if the etcd server completed the defrag despite this client-side error,
		// the independent auto-disalarm pass above will clear the NOSPACE alarm on the
		// next reconcile once the db size ratio has dropped below the threshold.
		log.Error(err, "Failed to defragment etcd member, will retry", "member", target.member.Name)
		return ctrl.Result{RequeueAfter: defragErrorRetryInterval}, nil
	}

	log.Info("Successfully defragmented etcd member", "member", target.member.Name)

	// Attempt auto-disalarm immediately after a successful defrag so the NOSPACE alarm
	// is cleared in the same reconcile cycle without waiting for the next pass.
	if spec.AutoDisalarm && memberHasNoSpaceAlarm(controlPlane.EtcdMembersAlarms, target.member.ID) {
		maybeDisarmNoSpaceAlarm(ctx, workloadCluster, target.member.Name, target.member.ID, disalarmThresholdRatio, etcdLocal)
	}

	// Stamp the per-member defrag time after a successful defragmentation.
	now := metav1.Now()
	setMemberDefragTime(&controlPlane.KCP.Status.EtcdMemberDefragTimes, target.member.Name, now)

	// If more members still need defragmentation, requeue after a short settling delay
	// so the next reconcile can re-evaluate each member's updated status.
	if len(candidates) > 1 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Even though no further candidates remain right now, a throttled member may become
	// eligible later. Schedule a requeue so it is not missed.
	if earliestRequeue > 0 {
		return ctrl.Result{RequeueAfter: earliestRequeue}, nil
	}

	// earliestRequeue was computed before the defrag timestamp was stamped above, so the
	// just-defragged member's new throttle is not yet reflected in earliestRequeue.
	// Schedule a requeue at minInterval so the next evaluation is not missed.
	return ctrl.Result{RequeueAfter: minInterval}, nil
}

// memberDefragTime returns the LastDefragTime for the named member from the status list,
// or nil if no entry exists for that member yet.
func memberDefragTime(times []controlplanev1.EtcdMemberDefragStatus, name string) *metav1.Time {
	for i := range times {
		if times[i].Name == name {
			return &times[i].LastDefragTime
		}
	}
	return nil
}

// maybeDisarmNoSpaceAlarm fetches fresh status for the named etcd member and clears its
// NOSPACE alarm if the db size ratio (dbSize/quota) is strictly below thresholdRatio.
// Errors are logged and swallowed; callers treat disalarm as best-effort.
func maybeDisarmNoSpaceAlarm(ctx context.Context, workloadCluster internal.WorkloadCluster, memberName string, memberID uint64, thresholdRatio float64, etcdLocal *bootstrapv1.LocalEtcd) {
	log := ctrl.LoggerFrom(ctx)

	freshStatus, err := workloadCluster.EtcdMemberStatus(ctx, memberName)
	if err != nil {
		log.Error(err, "Failed to get etcd member status for auto-disalarm check, skipping", "member", memberName)
		return
	}
	quota := resolveEtcdQuota(freshStatus.DbSizeQuota, etcdLocal)
	ratio := float64(freshStatus.DbSize) / float64(quota)
	if ratio < thresholdRatio {
		if err := workloadCluster.DisarmEtcdMemberNoSpaceAlarm(ctx, memberName, memberID); err != nil {
			log.Error(err, "Failed to disarm NOSPACE alarm on etcd member, skipping", "member", memberName)
		} else {
			log.Info("Disarmed NOSPACE alarm on etcd member", "member", memberName,
				"ratio", ratio, "threshold", thresholdRatio)
		}
	} else {
		log.Info("Skipping NOSPACE alarm disarm: db size ratio still above threshold",
			"member", memberName, "ratio", ratio, "threshold", thresholdRatio)
	}
}

// memberHasNoSpaceAlarm reports whether the given member ID has a NOSPACE alarm in the list.
func memberHasNoSpaceAlarm(alarms []etcd.MemberAlarm, memberID uint64) bool {
	for _, a := range alarms {
		if a.MemberID == memberID && a.Type == etcd.AlarmNoSpace {
			return true
		}
	}
	return false
}

// setMemberDefragTime upserts the defrag timestamp for the named member in the status list.
func setMemberDefragTime(times *[]controlplanev1.EtcdMemberDefragStatus, name string, t metav1.Time) {
	for i := range *times {
		if (*times)[i].Name == name {
			(*times)[i].LastDefragTime = t
			return
		}
	}
	*times = append(*times, controlplanev1.EtcdMemberDefragStatus{Name: name, LastDefragTime: t})
}

// defaultEtcdQuotaBytes is the etcd built-in default storage quota (2 GiB),
// used when the quota cannot be determined from the Status response or extraArgs.
const defaultEtcdQuotaBytes int64 = 2 * 1024 * 1024 * 1024

// resolveEtcdQuota returns the etcd storage quota in bytes for a given member,
// consulting the following sources in order:
//
//  1. DbSizeQuota from the etcd Status response (available since etcd v3.6).
//  2. quota-backend-bytes from spec.kubeadmConfigSpec.clusterConfiguration.etcd.local.extraArgs.
//  3. The etcd built-in default of 2 GiB.
//
// Note: step 2 is a compatibility shim for etcd v3.5, which does not expose quota in its
// Status response. It will be removed once etcd v3.5 is out of support.
func resolveEtcdQuota(dbSizeQuotaFromStatus int64, etcdLocal *bootstrapv1.LocalEtcd) int64 {
	// Step 1: DbSizeQuota is populated by etcd v3.6+ in the Status response.
	// On etcd v3.5 the field is absent and proto3 zero-decodes it as 0.
	if dbSizeQuotaFromStatus > 0 {
		return dbSizeQuotaFromStatus
	}

	// Step 2: fall back to parsing quota-backend-bytes from extraArgs.
	// TODO: Remove this block once etcd v3.5 is out of support.
	if etcdLocal != nil {
		for _, arg := range etcdLocal.ExtraArgs {
			if arg.Name == "quota-backend-bytes" {
				if arg.Value != nil {
					quota, err := strconv.ParseInt(*arg.Value, 10, 64)
					if err == nil && quota > 0 {
						return quota
					}
				}
				break
			}
		}
	}

	// Step 3: fall back to etcd's built-in default.
	return defaultEtcdQuotaBytes
}
