/*
Copyright 2022 The Kubernetes Authors.

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

package cmd

import (
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

type topologyDryRunOptions struct {
	kubeconfig        string
	kubeconfigContext string
	file              string
	cluster           string
	outDir            string
}

var dr = &topologyDryRunOptions{}

var topologyDryRunCmd = &cobra.Command{
	Use:   "topology-dryrun",
	Short: "Dry run changes to clusters that use managed topologies.",
	Long: LongDesc(`
		Provide the list of objects that would be created, modified and deleted when an input file is applied.
		The input can be a file with a new/modified cluster, new/modified ClusterClass, new/modified templates.
		Details about the objects that will be created and modified will be stored in a path passed using --output-directory.

		This command can also be run without a real cluster. In such cases, the input should contain all the objects needed
		to perform a dry run.

		Note: Among all the objects in the input defaulting and validation will be performed only for Cluster
		and ClusterClasses. All other objects in the input are expected to be valid and have default values.
	`),
	Example: Examples(`
		# List all the objects that will be created and modified when creating a new cluster.
		clusterctl alpha topology-dryrun -f new-cluster.yaml -o output/
	    
		# List the changes when modifying a cluster.
		clusterctl alpha topology-dryrun -f modified-cluster.yaml -o output/

		# List all the objects that will be created and modified when creating a new cluster along with a new ClusterClass.
		clusterctl alpha topology-dryrun -f new-cluster-and-cluster-class.yaml -o output/

		# List the clusters impacted by a ClusterClass change.
		clusterctl alpha topology-dryrun -f modified-cluster-class.yaml -o output/
	
		# List the changes to "cluster1" when a ClusterClass is changed.
		clusterctl alpha topology-dryrun -f modified-cluster-class.yaml --cluster "cluster1" -o output/

		# List the clusters and ClusterClasses impacted by a template change.
		clusterctl alpha topology-dryrun -f modified-template.yaml -o output/
	`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTopologyDryRun()
	},
}

func init() {
	topologyDryRunCmd.Flags().StringVar(&initOpts.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	topologyDryRunCmd.Flags().StringVar(&initOpts.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")

	topologyDryRunCmd.Flags().StringVarP(&dr.file, "file", "f", "", "path to the file with new or modified resources to be applied; the file should not contain more than one Cluster or more than one ClusterClass")
	topologyDryRunCmd.Flags().StringVarP(&dr.cluster, "cluster", "c", "", "name of the target cluster; this parameter is required when more than one cluster is affected")
	topologyDryRunCmd.Flags().StringVarP(&dr.outDir, "output-directory", "o", "", "output directory to write details about created/modified objects")

	if err := topologyDryRunCmd.MarkFlagRequired("file"); err != nil {
		panic(err)
	}
	if err := topologyDryRunCmd.MarkFlagRequired("output-directory"); err != nil {
		panic(err)
	}
}

func runTopologyDryRun() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(dr.file)
	if err != nil {
		return errors.Wrap(err, "failed to read input file")
	}

	objs, err := utilyaml.ToUnstructured(raw)
	if err != nil {
		return errors.Wrap(err, "failed to convert file to list of objects")
	}

	out, err := c.DryRunTopology(client.DryRunOptions{
		Kubeconfig: client.Kubeconfig{Path: dr.kubeconfig, Context: dr.kubeconfigContext},
		Objs:       convertToPtrSlice(objs),
		Cluster:    dr.cluster,
	})
	if err != nil {
		return err
	}
	return printDryRunOutput(out, dr.outDir)
}

func printDryRunOutput(out *cluster.DryRunOutput, outdir string) error {
	printAffectedClusterClasses(out)
	printAffectedClusters(out)
	if len(out.Clusters) == 0 {
		// No affected clusters. Return early.
		return nil
	}
	if out.ReconciledCluster.Name == "" {
		fmt.Printf("No target cluster identified. Use --cluster to specify a target cluster to get detailed changes.")
	} else {
		printChangeSummary(out)
		if err := writeOutputFiles(out, outdir); err != nil {
			return errors.Wrap(err, "failed to write output files of target cluster changes")
		}
	}
	fmt.Printf("\n")
	return nil
}

func printAffectedClusterClasses(out *cluster.DryRunOutput) {
	if len(out.ClusterClasses) == 0 {
		// If there are no affected ClusterClasses return early. Nothing more to do here.
		fmt.Printf("No ClusterClasses will be affected by the changes.\n")
		return
	}
	fmt.Printf("The following ClusterClasses will be affected by the changes:\n")
	for _, cc := range out.ClusterClasses {
		fmt.Printf(" ＊ %s/%s\n", cc.Namespace, cc.Name)
	}
	fmt.Printf("\n")
}

func printAffectedClusters(out *cluster.DryRunOutput) {
	if len(out.Clusters) == 0 {
		// if there are not affected Clusters return early. Nothing more to do here.
		fmt.Printf("No Clusters will be affected by the changes.\n")
		return
	}
	fmt.Printf("The following Clusters will be affected by the changes:\n")
	for _, cluster := range out.Clusters {
		fmt.Printf(" ＊ %s/%s\n", cluster.Namespace, cluster.Name)
	}
	fmt.Printf("\n")
}

func printChangeSummary(out *cluster.DryRunOutput) {
	if len(out.Created) == 0 && len(out.Modified) == 0 && len(out.Deleted) == 0 {
		fmt.Printf("No changes detected for Cluster %q.\n", fmt.Sprintf("%s/%s", out.ReconciledCluster.Namespace, out.ReconciledCluster.Name))
		return
	}

	fmt.Printf("Changes for Cluster %q: \n", fmt.Sprintf("%s/%s", out.ReconciledCluster.Namespace, out.ReconciledCluster.Name))
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Namespace", "Kind", "Name", "Action"})
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)

	// Add the created rows.
	sort.Slice(out.Created, func(i, j int) bool { return lessByKindAndName(out.Created[i], out.Created[j]) })
	for _, c := range out.Created {
		addRow(table, c, "created", tablewriter.FgGreenColor)
	}

	// Add the modified rows.
	sort.Slice(out.Modified, func(i, j int) bool { return lessByKindAndName(out.Modified[i].After, out.Modified[j].After) })
	for _, m := range out.Modified {
		addRow(table, m.After, "modified", tablewriter.FgYellowColor)
	}

	// Add the deleted rows.
	sort.Slice(out.Deleted, func(i, j int) bool { return lessByKindAndName(out.Deleted[i], out.Deleted[j]) })
	for _, d := range out.Deleted {
		addRow(table, d, "deleted", tablewriter.FgRedColor)
	}
	fmt.Printf("\n")
	table.Render()
	fmt.Printf("\n")
}

func writeOutputFiles(out *cluster.DryRunOutput, outDir string) error {
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		return fmt.Errorf("output directory %q does not exist", outDir)
	}

	// Write created files
	createdDir := path.Join(outDir, "created")
	if err := os.MkdirAll(createdDir, 0750); err != nil {
		return errors.Wrapf(err, "failed to create %q directory", createdDir)
	}
	for _, c := range out.Created {
		yaml, err := utilyaml.FromUnstructured([]unstructured.Unstructured{*c})
		if err != nil {
			return errors.Wrap(err, "failed to convert object to yaml")
		}
		fileName := fmt.Sprintf("%s_%s_%s.yaml", c.GetKind(), c.GetNamespace(), c.GetName())
		filePath := path.Join(createdDir, fileName)
		if err := os.WriteFile(filePath, yaml, 0600); err != nil {
			return errors.Wrapf(err, "failed to write yaml to file %q", filePath)
		}
	}
	if len(out.Created) != 0 {
		fmt.Printf("Created objects are written to directory %q\n", createdDir)
	}

	// Write modified files
	modifiedDir := path.Join(outDir, "modified")
	if err := os.MkdirAll(modifiedDir, 0750); err != nil {
		return errors.Wrapf(err, "failed to create %q directory", modifiedDir)
	}
	for _, m := range out.Modified {
		// Write the modified object to file.
		fileNameModified := fmt.Sprintf("%s_%s_%s.modified.yaml", m.After.GetKind(), m.After.GetNamespace(), m.After.GetName())
		filePathModified := path.Join(modifiedDir, fileNameModified)
		if err := writeObjectToFile(filePathModified, m.After); err != nil {
			return errors.Wrap(err, "failed to write modified object to file")
		}

		// Write the original object to file.
		fileNameOriginal := fmt.Sprintf("%s_%s_%s.original.yaml", m.Before.GetKind(), m.Before.GetNamespace(), m.Before.GetName())
		filePathOriginal := path.Join(modifiedDir, fileNameOriginal)
		if err := writeObjectToFile(filePathOriginal, m.Before); err != nil {
			return errors.Wrap(err, "failed to write original object to file")
		}

		// Calculate the diff and write to a file.
		patch := crclient.MergeFrom(m.Before)
		diff, err := patch.Data(m.After)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate diff of modified object %s/%s", m.After.GetNamespace(), m.After.GetName())
		}
		patchFileName := fmt.Sprintf("%s_%s_%s.patch", m.After.GetKind(), m.After.GetNamespace(), m.After.GetName())
		patchFilePath := path.Join(modifiedDir, patchFileName)
		if err := os.WriteFile(patchFilePath, diff, 0600); err != nil {
			return errors.Wrapf(err, "failed to write diff to file %q", patchFilePath)
		}
	}
	if len(out.Modified) != 0 {
		fmt.Printf("Modified objects are written to directory %q\n", modifiedDir)
	}

	return nil
}

func writeObjectToFile(filePath string, obj *unstructured.Unstructured) error {
	yaml, err := utilyaml.FromUnstructured([]unstructured.Unstructured{*obj})
	if err != nil {
		return errors.Wrap(err, "failed to convert object to yaml")
	}
	if err := os.WriteFile(filePath, yaml, 0600); err != nil {
		return errors.Wrapf(err, "failed to write yaml to file %q", filePath)
	}
	return nil
}

func convertToPtrSlice(objs []unstructured.Unstructured) []*unstructured.Unstructured {
	res := []*unstructured.Unstructured{}
	for i := range objs {
		res = append(res, &objs[i])
	}
	return res
}

func lessByKindAndName(a, b *unstructured.Unstructured) bool {
	if a.GetKind() == b.GetKind() {
		return a.GetName() < b.GetName()
	}
	return a.GetKind() < b.GetKind()
}

func addRow(table *tablewriter.Table, o *unstructured.Unstructured, action string, actionColor int) {
	table.Rich(
		[]string{
			o.GetNamespace(),
			o.GetKind(),
			o.GetName(),
			action,
		},
		[]tablewriter.Colors{
			{}, {}, {}, {actionColor},
		},
	)
}
