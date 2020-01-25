/*
Copyright 2020 The Kubernetes Authors.

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
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_objectMover_move(t *testing.T) {
	type fields struct {
		objs []runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Cluster",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "foo").Objs(),
			},
			wantErr: false,
		},
		{
			name: "Cluster with machine",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithMachines(
						test.NewFakeMachine("m1"),
						test.NewFakeMachine("m2"),
					).Objs(),
			},
			wantErr: false,
		},
		{
			name: "Cluster with MachineSet",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithMachineSets(
						test.NewFakeMachineSet("ms1").
							WithMachines(
								test.NewFakeMachine("m1"),
								test.NewFakeMachine("m2"),
							),
					).Objs(),
			},
			wantErr: false,
		},
		{
			name: "Cluster with MachineDeployment",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							),
					).Objs(),
			},
			wantErr: false,
		},
		{
			name: "Cluster with Control Plane",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp1").
							WithMachines(
								test.NewFakeMachine("m1"),
								test.NewFakeMachine("m2"),
							),
					).Objs(),
			},
			wantErr: false,
		},
		{
			name: "Cluster with MachinePool",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithMachinePools(
						test.NewFakeMachinePool("mp1"),
					).Objs(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			if err != nil {
				t.Fatal(err)
			}

			// trigger discovery the content of the source cluster
			if err := graph.Discovery("ns1", discoveryTypes); err != nil {
				t.Fatal(err)
			}

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy:     graph.proxy,
				machineWaiter: fakeMachineWaiter,
				log:           graph.log,
			}

			err = mover.move(graph, toProxy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// check that the objects are removed from the source cluster and are created in the target cluster
			csFrom, err := graph.proxy.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			csTo, err := toProxy.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			for _, node := range graph.uidToNode {
				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are deleted from the source cluster
				oFrom := &unstructured.Unstructured{}
				oFrom.SetAPIVersion(node.identity.APIVersion)
				oFrom.SetKind(node.identity.Kind)

				err := csFrom.Get(ctx, key, oFrom)
				if err == nil {
					t.Errorf("%v not deleted in source cluster", key)
					continue
				}
				if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %v deleted in source cluster", err, key)
					continue
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %v created in target cluster", err, key)
					continue
				}
			}
		})
	}
}

func fakeMachineWaiter(proxy Proxy, machineKey client.ObjectKey) error {
	return nil
}
