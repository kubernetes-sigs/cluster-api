// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package digitalocean

import (
	"fmt"
	"github.com/kris-nova/charlie/network"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/logger"
	profile "github.com/kris-nova/kubicorn/profiles/digitalocean"
	"github.com/kris-nova/kubicorn/test"
	"os"
	"testing"
	"time"
)

var testCluster *cluster.Cluster

func TestMain(m *testing.M) {
	logger.TestMode = true
	logger.Level = 4
	var err error
	//func() {
	//	for {
	//		if r := recover(); r != nil {
	//			logger.Critical("Panic: %v", r)
	//		}
	//	}
	//}()
	test.InitRsaTravis()
	testCluster = profile.NewCentosCluster("centos-test")
	testCluster, err = test.Create(testCluster)
	if err != nil {
		fmt.Printf("Unable to create DigitalOcean test cluster: %v\n", err)
		os.Exit(1)
	}
	status := m.Run()
	exitCode := 0
	if status != 0 {
		fmt.Printf("-----------------------------------------------------------------------\n")
		fmt.Printf("[FAILURE]\n")
		fmt.Printf("-----------------------------------------------------------------------\n")
		exitCode = 1
	}
	_, err = test.Delete(testCluster)
	if err != nil {
		exitCode = 99
		fmt.Println("Failure cleaning up cluster! Abandoned resources!")
	}
	os.Exit(exitCode)
}

const (
	APISleepSeconds   = 5
	APISocketAttempts = 40
)

func TestApiListen(t *testing.T) {
	success := false
	for i := 0; i < APISocketAttempts; i++ {
		_, err := network.AssertTcpSocketAcceptsConnection(fmt.Sprintf("%s:%s", testCluster.KubernetesAPI.Endpoint, testCluster.KubernetesAPI.Port), "opening a new socket connection against the Kubernetes API")
		if err != nil {
			logger.Info("Attempting to open a socket to the Kubernetes API: %v...\n", err)
			time.Sleep(time.Duration(APISleepSeconds) * time.Second)
			continue
		}
		success = true
	}
	if !success {
		t.Fatalf("Unable to connect to Kubernetes API")
	}
}
