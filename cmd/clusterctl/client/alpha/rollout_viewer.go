/*
Copyright 2023 The Kubernetes Authors.

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

package alpha

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

type historyInfo struct {
	revisions   string
	changeCause string
}

// ObjectViewer will issue a view on the specified cluster-api resource.
func (r *rollout) ObjectViewer(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference, revision int64) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(ctx, proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to get %v/%v", ref.Kind, ref.Name)
		}
		if err := viewMachineDeployment(ctx, proxy, deployment, revision); err != nil {
			return err
		}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validHistoryResourceTypes)
	}
	return nil
}

func viewMachineDeployment(ctx context.Context, proxy cluster.Proxy, d *clusterv1.MachineDeployment, revision int64) error {
	log := logf.Log
	msList, err := getMachineSetsForDeployment(ctx, proxy, d)
	if err != nil {
		return err
	}

	if revision < 0 {
		return errors.Errorf("revision number cannot be negative: %v", revision)
	}

	// Print details of a specific revision
	if revision > 0 {
		ms, err := findMachineDeploymentRevision(revision, msList)
		if err != nil {
			return errors.Errorf("unable to find the specified revision %d for MachineDeployment %s", revision, d.Name)
		}
		output, err := yaml.Marshal(ms.Spec.Template)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, string(output))
		return nil
	}

	// Print an overview of all revisions
	// Create a revisionToChangeCause map
	histInfo := make(map[int64]historyInfo)
	for _, ms := range msList {
		v, err := mdutil.Revision(ms)
		if err != nil {
			log.Error(err, fmt.Sprintf("unable to get revision from machineset %s for machinedeployment %s in namespace %s", ms.Name, d.Name, d.Namespace))
			continue
		}
		revisions := strconv.FormatInt(v, 10)
		if revHistory := ms.Annotations[clusterv1.RevisionHistoryAnnotation]; revHistory != "" {
			revisions = revHistory + "," + revisions
		}
		histInfo[v] = historyInfo{
			revisions,
			ms.Annotations[clusterv1.ChangeCauseAnnotation],
		}
	}

	// Sort the revisions
	revisions := make([]int64, 0, len(histInfo))
	for r := range histInfo {
		revisions = append(revisions, r)
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i] < revisions[j] })

	// Output the revisionToChangeCause map
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"REVISIONS", "CHANGE-CAUSE"})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)

	for _, r := range revisions {
		changeCause := histInfo[r].changeCause
		if changeCause == "" {
			changeCause = "<none>"
		}
		table.Append([]string{
			histInfo[r].revisions,
			changeCause,
		})
	}
	table.Render()

	return nil
}
