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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
)

func PopulateDefaultsMachineDeployment(d *MachineDeployment) {
	if d.Spec.Strategy == nil {
		d.Spec.Strategy = &MachineDeploymentStrategy{}
	}
	if d.Spec.Strategy.Type == "" {
		d.Spec.Strategy.Type = common.RollingUpdateMachineDeploymentStrategyType
	}

	if d.Spec.Strategy.Type == common.RollingUpdateMachineDeploymentStrategyType {
		if d.Spec.Strategy.RollingUpdate == nil {
			d.Spec.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
		}
		if d.Spec.Strategy.RollingUpdate.MaxSurge == nil {
			ios1 := intstr.FromInt(1)
			d.Spec.Strategy.RollingUpdate.MaxSurge = &ios1
		}
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable == nil {
			ios0 := intstr.FromInt(0)
			d.Spec.Strategy.RollingUpdate.MaxUnavailable = &ios0
		}
	}

	if d.Spec.MinReadySeconds == nil {
		v := int32(0)
		d.Spec.MinReadySeconds = &v
	}
}
