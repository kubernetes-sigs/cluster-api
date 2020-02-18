// +build e2e

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
	"context"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ClusterCtl E2E Suite")
}

const (
	setupTimeout = 10 * 60
)

var (
	ctx                  context.Context
	managerImage         string
	kindConfigFile       string
	clusterctlConfigFile string
)

var _ = BeforeSuite(func() {
	ctx = context.Background()
	// Docker image to load into the kind cluster for testing
	managerImage = os.Getenv("MANAGER_IMAGE")
	if managerImage == "" {
		fmt.Fprintf(GinkgoWriter, "MANAGER_IMAGE not specified, using default %v\n", "gcr.io/k8s-staging-capi-docker/capd-manager-amd64:dev")
		managerImage = "gcr.io/k8s-staging-capi-docker/capd-manager-amd64:dev"
	} else {
		fmt.Fprintf(GinkgoWriter, "Using MANAGER_IMAGE %v\n", managerImage)
	}
	kindConfigFile = os.Getenv("KIND_CONFIG_FILE")
	if kindConfigFile == "" {
		fmt.Fprintf(GinkgoWriter, "KIND_CONFIG_FILE not found, capd mgmt cluster wil be created without any special kind configuration.\n")
	} else {
		fmt.Fprintf(GinkgoWriter, "Using KIND_CONFIG_FILE: %v\n", kindConfigFile)
	}
	clusterctlConfigFile = os.Getenv("CLUSTERCTL_CONFIG")
	if clusterctlConfigFile == "" {
		fmt.Fprintf(GinkgoWriter, "CLUSTERCTL_CONFIG not found.\n")
	} else {
		fmt.Fprintf(GinkgoWriter, "Using CLUSTERCTL_CONFIG: %v\n", clusterctlConfigFile)
	}
}, setupTimeout)

var _ = AfterSuite(func() {
})
