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

package cluster

import (
	"context"

	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
)

func (r *Reconciler) mitigateManagedFieldsIssue(ctx context.Context, s *scope.Scope) (bool, error) {
	if s.Current == nil {
		return false, nil
	}

	anyManagedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, s.Current.Cluster, structuredmerge.TopologyManagerName)
	if err != nil {
		return false, err
	}

	managedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, s.Current.InfrastructureCluster, structuredmerge.TopologyManagerName)
	if err != nil {
		return false, err
	}
	anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

	if s.Current.ControlPlane != nil {
		managedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, s.Current.ControlPlane.MachineHealthCheck, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, s.Current.ControlPlane.Object, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, s.Current.ControlPlane.InfrastructureMachineTemplate, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated
	}

	for _, md := range s.Current.MachineDeployments {
		managedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, md.MachineHealthCheck, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, md.Object, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, md.InfrastructureMachineTemplate, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, md.BootstrapTemplate, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated
	}

	for _, mp := range s.Current.MachinePools {
		managedFieldIssueMitigated, err := ssa.MitigateManagedFieldsIssue(ctx, r.Client, mp.Object, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, mp.InfrastructureMachinePoolObject, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated

		managedFieldIssueMitigated, err = ssa.MitigateManagedFieldsIssue(ctx, r.Client, mp.BootstrapObject, structuredmerge.TopologyManagerName)
		if err != nil {
			return false, err
		}
		anyManagedFieldIssueMitigated = anyManagedFieldIssueMitigated || managedFieldIssueMitigated
	}

	return anyManagedFieldIssueMitigated, nil
}
