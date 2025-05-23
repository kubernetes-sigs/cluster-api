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

package machinehealthcheck

import (
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

// MatchMachineHealthCheckStatus returns a custom matcher to check equality of clusterv1.MachineHealthCheckStatus.
func MatchMachineHealthCheckStatus(expected *clusterv1.MachineHealthCheckStatus) types.GomegaMatcher {
	return &machineHealthCheckStatusMatcher{
		expected: expected,
	}
}

type machineHealthCheckStatusMatcher struct {
	expected *clusterv1.MachineHealthCheckStatus
}

func (m machineHealthCheckStatusMatcher) Match(actual interface{}) (success bool, err error) {
	actualStatus, ok := actual.(*clusterv1.MachineHealthCheckStatus)
	if !ok {
		return false, fmt.Errorf("actual should be of type MachineHealthCheckStatus")
	}

	ok, err = Equal(m.expected.CurrentHealthy).Match(actualStatus.CurrentHealthy)
	if !ok {
		return ok, err
	}
	ok, err = Equal(m.expected.ExpectedMachines).Match(actualStatus.ExpectedMachines)
	if !ok {
		return ok, err
	}
	ok, err = Equal(m.expected.RemediationsAllowed).Match(actualStatus.RemediationsAllowed)
	if !ok {
		return ok, err
	}
	ok, err = Equal(m.expected.Targets).Match(actualStatus.Targets)
	if !ok {
		return ok, err
	}
	var mConditions clusterv1.Conditions
	if m.expected.Deprecated != nil && m.expected.Deprecated.V1Beta1 != nil {
		mConditions = m.expected.Deprecated.V1Beta1.Conditions
	}
	var actualConditions clusterv1.Conditions
	if actualStatus.Deprecated != nil && actualStatus.Deprecated.V1Beta1 != nil {
		actualConditions = actualStatus.Deprecated.V1Beta1.Conditions
	}
	ok, err = v1beta1conditions.MatchConditions(mConditions).Match(actualConditions)
	return ok, err
}

func (m machineHealthCheckStatusMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto match\n\t%#v\n", actual, m.expected)
}

func (m machineHealthCheckStatusMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto not match\n\t%#v\n", actual, m.expected)
}
