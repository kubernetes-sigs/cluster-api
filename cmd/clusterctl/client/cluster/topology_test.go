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

	//go:embed assets/topology-test/mock-CRDs.yaml
	mockCRDsYAML []byte

	//go:embed assets/topology-test/my-cluster-class.yaml
	existingMyClusterClassYAML []byte

	//go:embed assets/topology-test/existing-my-cluster.yaml
	existingMyClusterYAML []byte

	//go:embed assets/topology-test/existing-my-second-cluster.yaml
	existingMySecondClusterYAML []byte

	// modifiedClusterYAML changes the control plane replicas from 1 to 3.
	//go:embed assets/topology-test/modified-my-cluster.yaml
	modifiedMyClusterYAML []byte

	// modifiedDockerMachineTemplateYAML adds metadat to the docker machine used by the control plane template..
	//go:embed assets/topology-test/modified-CP-dockermachinetemplate.yaml
	modifiedDockerMachineTemplateYAML []byte

	//go:embed assets/topology-test/objects-in-different-namespaces.yaml
	objsInDifferentNamespacesYAML []byte
)

func Test_topologyClient_Plan(t *testing.T) {
	type args struct {
		in *TopologyPlanInput
	}
	type item struct {
		kind       string
		namespace  string
		namePrefix string
	}
	type out struct {
		affectedClusters       []client.ObjectKey
		affectedClusterClasses []client.ObjectKey
		reconciledCluster      *client.ObjectKey
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
				in: &TopologyPlanInput{
					Objs: mustToUnstructured(newClusterClassAndClusterYAML),
				},
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
					cluster := client.ObjectKey{Namespace: "default", Name: "my-cluster"}
					return []client.ObjectKey{cluster}
				}(),
				affectedClusterClasses: func() []client.ObjectKey {
					cc := client.ObjectKey{Namespace: "default", Name: "my-cluster-class"}
					return []client.ObjectKey{cc}
				}(),
				reconciledCluster: &client.ObjectKey{Namespace: "default", Name: "my-cluster"},
			},
			wantErr: false,
		},
		{
			name: "Modifying an existing Cluster",
			existingObjects: mustToUnstructured(
				mockCRDsYAML,
				existingMyClusterClassYAML,
				existingMyClusterYAML,
			),
			args: args{
				in: &TopologyPlanInput{
					Objs: mustToUnstructured(modifiedMyClusterYAML),
				},
			},
			want: out{
				affectedClusters: func() []client.ObjectKey {
					cluster := client.ObjectKey{Namespace: "default", Name: "my-cluster"}
					return []client.ObjectKey{cluster}
				}(),
				affectedClusterClasses: []client.ObjectKey{},
				modified: []item{
					{kind: "KubeadmControlPlane", namespace: "default", namePrefix: "my-cluster-"},
				},
				reconciledCluster: &client.ObjectKey{Namespace: "default", Name: "my-cluster"},
			},
			wantErr: false,
		},
		{
			name: "Modifying an existing DockerMachineTemplate. Template used by Control Plane of an existing Cluster.",
			existingObjects: mustToUnstructured(
				mockCRDsYAML,
				existingMyClusterClassYAML,
				existingMyClusterYAML,
			),
			args: args{
				in: &TopologyPlanInput{
					Objs: mustToUnstructured(modifiedDockerMachineTemplateYAML),
				},
			},
			want: out{
				affectedClusters: func() []client.ObjectKey {
					cluster := client.ObjectKey{Namespace: "default", Name: "my-cluster"}
					return []client.ObjectKey{cluster}
				}(),
				affectedClusterClasses: func() []client.ObjectKey {
					cc := client.ObjectKey{Namespace: "default", Name: "my-cluster-class"}
					return []client.ObjectKey{cc}
				}(),
				modified: []item{
					{kind: "KubeadmControlPlane", namespace: "default", namePrefix: "my-cluster-"},
				},
				created: []item{
					// Modifying the DockerClusterTemplate will result in template rotation. A new template will be created
					// and used by KCP.
					{kind: "DockerMachineTemplate", namespace: "default", namePrefix: "my-cluster-control-plane-"},
				},
				reconciledCluster: &client.ObjectKey{Namespace: "default", Name: "my-cluster"},
			},
			wantErr: false,
		},
		{
			name: "Modifying an existing DockerMachineTemplate. Affects multiple clusters. Target Cluster not specified.",
			existingObjects: mustToUnstructured(
				mockCRDsYAML,
				existingMyClusterClassYAML,
				existingMyClusterYAML,
				existingMySecondClusterYAML,
			),
			args: args{
				in: &TopologyPlanInput{
					Objs: mustToUnstructured(modifiedDockerMachineTemplateYAML),
				},
			},
			want: out{
				affectedClusters: func() []client.ObjectKey {
					cluster := client.ObjectKey{Namespace: "default", Name: "my-cluster"}
					cluster2 := client.ObjectKey{Namespace: "default", Name: "my-second-cluster"}
					return []client.ObjectKey{cluster, cluster2}
				}(),
				affectedClusterClasses: func() []client.ObjectKey {
					cc := client.ObjectKey{Namespace: "default", Name: "my-cluster-class"}
					return []client.ObjectKey{cc}
				}(),
				modified:          []item{},
				created:           []item{},
				reconciledCluster: nil,
			},
			wantErr: false,
		},
		{
			name: "Modifying an existing DockerMachineTemplate. Affects multiple clusters. Target Cluster specified.",
			existingObjects: mustToUnstructured(
				mockCRDsYAML,
				existingMyClusterClassYAML,
				existingMyClusterYAML,
				existingMySecondClusterYAML,
			),
			args: args{
				in: &TopologyPlanInput{
					Objs:              mustToUnstructured(modifiedDockerMachineTemplateYAML),
					TargetClusterName: "my-cluster",
				},
			},
			want: out{
				affectedClusters: func() []client.ObjectKey {
					cluster := client.ObjectKey{Namespace: "default", Name: "my-cluster"}
					cluster2 := client.ObjectKey{Namespace: "default", Name: "my-second-cluster"}
					return []client.ObjectKey{cluster, cluster2}
				}(),
				affectedClusterClasses: func() []client.ObjectKey {
					cc := client.ObjectKey{Namespace: "default", Name: "my-cluster-class"}
					return []client.ObjectKey{cc}
				}(),
				modified: []item{
					{kind: "KubeadmControlPlane", namespace: "default", namePrefix: "my-cluster-"},
				},
				created: []item{
					// Modifying the DockerClusterTemplate will result in template rotation. A new template will be created
					// and used by KCP.
					{kind: "DockerMachineTemplate", namespace: "default", namePrefix: "my-cluster-control-plane-"},
				},
				reconciledCluster: &client.ObjectKey{Namespace: "default", Name: "my-cluster"},
			},
			wantErr: false,
		},
		{
			name: "Input with objects in different namespaces should return error",
			args: args{
				in: &TopologyPlanInput{
					Objs: mustToUnstructured(objsInDifferentNamespacesYAML),
				},
			},
			wantErr: true,
		},
		{
			name: "Input with TargetNamespace different from objects in input should return error",
			args: args{
				in: &TopologyPlanInput{
					Objs:            mustToUnstructured(newClusterClassAndClusterYAML),
					TargetNamespace: "different-namespace",
				},
			},
			wantErr: true,
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

			res, err := tc.Plan(tt.args.in)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			// The plan should function should not return any error.
			g.Expect(err).NotTo(HaveOccurred())

			// Check affected ClusterClasses.
			g.Expect(len(res.ClusterClasses)).To(Equal(len(tt.want.affectedClusterClasses)))
			for _, cc := range tt.want.affectedClusterClasses {
				g.Expect(res.ClusterClasses).To(ContainElement(cc))
			}

			// Check affected Clusters.
			g.Expect(len(res.Clusters)).To(Equal(len(tt.want.affectedClusters)))
			for _, cluster := range tt.want.affectedClusters {
				g.Expect(res.Clusters).To(ContainElement(cluster))
			}

			// Check the reconciled cluster.
			if tt.want.reconciledCluster == nil {
				g.Expect(res.ReconciledCluster).To(BeNil())
			} else {
				g.Expect(res.ReconciledCluster).NotTo(BeNil())
				g.Expect(*res.ReconciledCluster).To(Equal(*tt.want.reconciledCluster))
			}

			// Check the created objects.
			for _, created := range tt.want.created {
				g.Expect(res.Created).To(ContainElement(MatchTopologyPlanOutputItem(created.kind, created.namespace, created.namePrefix)))
			}

			// Check the modified objects.
			actualModifiedObjs := []*unstructured.Unstructured{}
			for _, m := range res.Modified {
				actualModifiedObjs = append(actualModifiedObjs, m.After)
			}
			for _, modified := range tt.want.modified {
				g.Expect(actualModifiedObjs).To(ContainElement(MatchTopologyPlanOutputItem(modified.kind, modified.namespace, modified.namePrefix)))
			}

			// Check the deleted objects.
			for _, deleted := range tt.want.deleted {
				g.Expect(res.Deleted).To(ContainElement(MatchTopologyPlanOutputItem(deleted.kind, deleted.namespace, deleted.namePrefix)))
			}
		})
	}
}

func MatchTopologyPlanOutputItem(kind, namespace, namePrefix string) types.GomegaMatcher {
	return &topologyPlanOutputItemMatcher{kind, namespace, namePrefix}
}

type topologyPlanOutputItemMatcher struct {
	kind       string
	namespace  string
	namePrefix string
}

func (m *topologyPlanOutputItemMatcher) Match(actual interface{}) (bool, error) {
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

func (m *topologyPlanOutputItemMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected item Kind=%s, Namespace=%s, Name(prefix)=%s to be present", m.kind, m.namespace, m.namePrefix)
}

func (m *topologyPlanOutputItemMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected item Kind=%s, Namespace=%s, Name(prefix)=%s not to be present", m.kind, m.namespace, m.namePrefix)
}

func convertToPtrSlice(objs []unstructured.Unstructured) []*unstructured.Unstructured {
	res := []*unstructured.Unstructured{}
	for i := range objs {
		res = append(res, &objs[i])
	}
	return res
}

func mustToUnstructured(rawyamls ...[]byte) []*unstructured.Unstructured {
	objects := []unstructured.Unstructured{}
	for _, raw := range rawyamls {
		objs, err := utilyaml.ToUnstructured(raw)
		if err != nil {
			panic(err)
		}
		objects = append(objects, objs...)
	}
	return convertToPtrSlice(objects)
}
