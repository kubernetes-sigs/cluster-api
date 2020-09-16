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

package e2e

import (
	"fmt"

	"github.com/blang/semver"
	"github.com/onsi/gomega/types"
	"sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
)

// Test suite constants for e2e config variables
const (
	KubernetesVersion            = "KUBERNETES_VERSION"
	CNIPath                      = "CNI"
	CNIResources                 = "CNI_RESOURCES"
	KubernetesVersionUpgradeFrom = "KUBERNETES_VERSION_UPGRADE_FROM"
	KubernetesVersionUpgradeTo   = "KUBERNETES_VERSION_UPGRADE_TO"
	EtcdVersionUpgradeTo         = "ETCD_VERSION_UPGRADE_TO"
	CoreDNSVersionUpgradeTo      = "COREDNS_VERSION_UPGRADE_TO"
)

// Byf is deprecated. Use "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions" as dot import instead.
func Byf(format string, a ...interface{}) {
	ginkgoextensions.Byf(format, a...)
}

// HaveValidVersion succeeds if version is a valid semver version
func HaveValidVersion(version string) types.GomegaMatcher {
	return &validVersionMatcher{version: version}
}

type validVersionMatcher struct{ version string }

func (m *validVersionMatcher) Match(actual interface{}) (success bool, err error) {
	if _, err := semver.ParseTolerant(m.version); err != nil {
		return false, err
	}
	return true, nil
}

func (m *validVersionMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " to be a valid version ")
}

func (m *validVersionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " not to be a valid version ")
}
