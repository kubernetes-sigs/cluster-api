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

package cluster

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

var (
	//go:embed assets/topology-test/new-clusterclass-and-cluster.yaml
	newClusterClassAndClusterYAML []byte

	//go:embed assets/topology-test/existing-clusterclass-and-cluster.yaml
	existingClusterClassAndClusterYAML []byte

	// modifiedClusterYAML changes the control plane replicas from 1 to 3.
	//go:embed assets/topology-test/modified-cluster.yaml
	modifiedClusterYAML []byte
)

func Test_topologyClient_DryRun(t *testing.T) {
	type args struct {
		in *DryRunInput
	}
	type item struct {
		kind       string
		namespace  string
		namePrefix string
	}
	type out struct {
		affectedClusters       []client.ObjectKey
		affectedClusterClasses []client.ObjectKey
		created                []item
		modified               []item
		deleted                []item
	}
	tests := []struct {
		name            string
		existingObjects []*unstructured.Unstructured
		args            args
		want            out
		wantErr         bool
	}{
		{
			name: "Input with new ClusterClass and new Cluster",
			args: args{
				in: func() *DryRunInput {
					objs, err := utilyaml.ToUnstructured(newClusterClassAndClusterYAML)
					if err != nil {
						panic(err)
					}
					return &DryRunInput{
						Objs: convertToPtrSlice(objs),
					}
				}(),
			},
			want: out{
				created: []item{
					{kind: "DockerCluster", namespace: "default", namePrefix: "my-cluster-"},
					{kind: "DockerMachineTemplate", namespace: "default", namePrefix: "my-cluster-md-0-"},
					{kind: "DockerMachineTemplate", namespace: "default", namePrefix: "my-cluster-md-1-"},
					{kind: "DockerMachineTemplate", namespace: "default", namePrefix: "my-cluster-control-plane-"},
					{kind: "KubeadmConfigTemplate", namespace: "default", namePrefix: "my-cluster-md-0-bootstrap-"},
					{kind: "KubeadmConfigTemplate", namespace: "default", namePrefix: "my-cluster-md-1-bootstrap-"},
					{kind: "KubeadmControlPlane", namespace: "default", namePrefix: "my-cluster-"},
					{kind: "MachineDeployment", namespace: "default", namePrefix: "my-cluster-md-0-"},
					{kind: "MachineDeployment", namespace: "default", namePrefix: "my-cluster-md-1-"},
				},
				modified: []item{
					{kind: "Cluster", namespace: "default", namePrefix: "my-cluster"},
				},
				affectedClusters: func() []client.ObjectKey {
					cluster := client.ObjectKey{}
					cluster.Namespace = "default"
					cluster.Name = "my-cluster"
					return []client.ObjectKey{cluster}
				}(),
				affectedClusterClasses: func() []client.ObjectKey {
					cc := client.ObjectKey{}
					cc.Namespace = "default"
					cc.Name = "my-cluster-class"
					return []client.ObjectKey{cc}
				}(),
			},
			wantErr: false,
		},
		{
			name: "Modifying an existing Cluster",
			existingObjects: func() []*unstructured.Unstructured {
				objs, err := utilyaml.ToUnstructured(existingClusterClassAndClusterYAML)
				if err != nil {
					panic(err)
				}
				return convertToPtrSlice(objs)
			}(),
			args: args{
				in: func() *DryRunInput {
					objs, err := utilyaml.ToUnstructured(modifiedClusterYAML)
					if err != nil {
						panic(err)
					}
					return &DryRunInput{
						Objs: convertToPtrSlice(objs),
					}
				}(),
			},
			want: out{
				modified: []item{
					{kind: "KubeadmControlPlane", namespace: "default", namePrefix: "my-cluster-"},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			existingObjects := []client.Object{}
			for _, o := range tt.existingObjects {
				existingObjects = append(existingObjects, o)
			}
			proxy := test.NewFakeProxy().WithClusterAvailable(true).WithFakeCAPISetup().WithObjs(existingObjects...)
			inventoryClient := newInventoryClient(proxy, nil)
			tc := newTopologyClient(
				proxy,
				inventoryClient,
			)

			res, err := tc.DryRun(tt.args.in)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			for _, cc := range tt.want.affectedClusterClasses {
				g.Expect(res.ClusterClasses).To(ContainElement(cc))
			}
			for _, cluster := range tt.want.affectedClusters {
				g.Expect(res.Clusters).To(ContainElement(cluster))
			}
			for _, created := range tt.want.created {
				g.Expect(res.Created).To(ContainElement(MatchDryRunOutputItem(created.kind, created.namespace, created.namePrefix)))
			}
			actualModifiedObjs := []*unstructured.Unstructured{}
			for _, m := range res.Modified {
				actualModifiedObjs = append(actualModifiedObjs, m.After)
			}
			for _, modified := range tt.want.modified {
				g.Expect(actualModifiedObjs).To(ContainElement(MatchDryRunOutputItem(modified.kind, modified.namespace, modified.namePrefix)))
			}
			for _, deleted := range tt.want.deleted {
				g.Expect(res.Deleted).To(ContainElement(MatchDryRunOutputItem(deleted.kind, deleted.namespace, deleted.namePrefix)))
			}
		})
	}
}

func MatchDryRunOutputItem(kind, namespace, namePrefix string) types.GomegaMatcher {
	return &dryRunOutputItemMatcher{kind, namespace, namePrefix}
}

type dryRunOutputItemMatcher struct {
	kind       string
	namespace  string
	namePrefix string
}

func (m *dryRunOutputItemMatcher) Match(actual interface{}) (bool, error) {
	obj := actual.(*unstructured.Unstructured)
	if obj.GetKind() != m.kind {
		return false, nil
	}
	if obj.GetNamespace() != m.namespace {
		return false, nil
	}
	if !strings.HasPrefix(obj.GetName(), m.namePrefix) {
		return false, nil
	}
	return true, nil
}

func (m *dryRunOutputItemMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected item Kind=%s, Namespace=%s, Name(prefix)=%s to be present", m.kind, m.namespace, m.namePrefix)
}

func (m *dryRunOutputItemMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected item Kind=%s, Namespace=%s, Name(prefix)=%s not to be present", m.kind, m.namespace, m.namePrefix)
}

func convertToPtrSlice(objs []unstructured.Unstructured) []*unstructured.Unstructured {
	res := []*unstructured.Unstructured{}
	for i := range objs {
		res = append(res, &objs[i])
	}
	return res
}
