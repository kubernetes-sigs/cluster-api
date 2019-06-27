/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"testing"
)

type myclient struct{
	statusWriter client.StatusWriter
	md *v1alpha1.MachineDeployment
	ms *v1alpha1.MachineSet
}
func (m *myclient) Create(ctx context.Context, obj runtime.Object) error {
	set, ok := obj.(*v1alpha1.MachineSet)
	if !ok {return errors.New("Not a machine set")}
	m.ms = set
	return nil
}
func (m *myclient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOptionFunc) error {return nil}
func (m *myclient) Update(ctx context.Context, obj runtime.Object) error {
	dep, ok := obj.(*v1alpha1.MachineDeployment)
	if !ok { return errors.New("not a MachineDeployment")}
	m.md = dep
	return nil
}
func (m *myclient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {return nil}
func (m *myclient) List(ctx context.Context, opts *client.ListOptions, list runtime.Object) error {return nil}
func (m *myclient) Status() client.StatusWriter {return m.statusWriter}

type mystatus struct {}
func (m *mystatus) Update(ctx context.Context, obj runtime.Object) error {return nil}

func TestRollingRolloutHasStaleMachineSet(t *testing.T) {
	c := &myclient{
		statusWriter: &mystatus{},
	}
	r := ReconcileMachineDeployment{
		Client: c,
	}
	md := simpleMD()
	md.Spec.Template.Spec.Versions.Kubelet = "v1.14.1"

	ms := simpleMS()
	ms.UID = "testuid"
	msl := []*v1alpha1.MachineSet{ms}
	mm := map[types.UID]*v1alpha1.MachineList{
		types.UID("testuid"): {
			Items: []v1alpha1.Machine{
				{
					Spec:v1alpha1.MachineSpec{
						Versions:v1alpha1.MachineVersionInfo{
							Kubelet:"v1.13.0",
						},
					},
				},
			},
		},
	}
	err := r.rolloutRolling(md, msl, mm)
	if err != nil {
		t.Fatal(err)
	}
	if len(md.Annotations) == 0 {
		t.Fatal("Expected an annotation")
	}
	if *c.ms.Spec.Replicas != 3 {
		t.Fatal("the new ms should have 3 replicas, not 1")
	}
}


func TestRollingRolloutHasUpToDateMachineSet(t *testing.T) {
	r := ReconcileMachineDeployment{
		Client: &myclient{
			statusWriter: &mystatus{},
		},
	}
	md := simpleMD()
	msl := []*v1alpha1.MachineSet{
		simpleMS(),
	}
	mm := map[types.UID]*v1alpha1.MachineList{}
	err := r.rolloutRolling(md, msl, mm)
	if err != nil {
		t.Fatal(err)
	}
}

func simpleMD() *v1alpha1.MachineDeployment {
	replicas := int32(3)
	minReadySeconds := int32(10)

	return &v1alpha1.MachineDeployment{
		Spec: v1alpha1.MachineDeploymentSpec{
		MinReadySeconds: &minReadySeconds,
		Selector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"test": "test",
			},
		},
		Replicas: &replicas,
		Strategy: &v1alpha1.MachineDeploymentStrategy{
			Type: common.RollingUpdateMachineDeploymentStrategyType,
			RollingUpdate: &v1alpha1.MachineRollingUpdateDeployment{
				MaxSurge: &intstr.IntOrString{
					IntVal: int32(1),
				},
			},
		},
		Template: v1alpha1.MachineTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"test": "test",
				},
			},
			Spec: v1alpha1.MachineSpec{
				Versions: v1alpha1.MachineVersionInfo{
					Kubelet: "v1.13.0",
				},
			},
		},
	},

}
}

func simpleMS() *v1alpha1.MachineSet {
	replicas := int32(3)
	return &v1alpha1.MachineSet{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"test": "test",
		},
	},
		Spec: v1alpha1.MachineSetSpec{
			Replicas:&replicas,
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"test": "test",
					},
				},
				Spec: v1alpha1.MachineSpec{
					Versions:v1alpha1.MachineVersionInfo{
						Kubelet: "v1.13.0",
					},
				},
			},
		},
	}
}