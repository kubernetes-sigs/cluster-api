/*
Copyright 2019 The Kubernetes Authors.

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

package generator

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

type MachineGenerator struct {
	Client client.Client
}

func NewMachineGenerator(client client.Client) *MachineGenerator {
	return &MachineGenerator{Client: client}
}

type MachineGeneratorInput struct {
	Namespace    string
	NamePrefix   string
	ClusterName  string
	Version      *string
	InfraRef     corev1.ObjectReference
	BootstrapRef *corev1.ObjectReference
	Labels       map[string]string
	Owner        *metav1.OwnerReference
}

func (mgg *MachineGenerator) GenerateMachine(ctx context.Context, input MachineGeneratorInput) error {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", input.NamePrefix),
			Labels:       input.Labels,
			Namespace:    input.Namespace,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       input.ClusterName,
			Version:           input.Version,
			InfrastructureRef: input.InfraRef,
		},
	}

	if input.BootstrapRef != nil {
		machine.Spec.Bootstrap = clusterv1.Bootstrap{ConfigRef: input.BootstrapRef}
	}

	if input.Owner != nil {
		machine.SetOwnerReferences([]metav1.OwnerReference{*input.Owner})
	}

	if err := mgg.Client.Create(ctx, machine); err != nil {
		return errors.Wrap(err, "Failed to create machine")
	}

	return nil
}
