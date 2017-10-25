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

package cloud

import (
	"os"
	"strings"
	"time"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/hang"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/signals"
)

var sigCaught = false
var sigHandler *signals.Handler

type AtomicReconciler struct {
	known *cluster.Cluster
	model Model
}

func NewAtomicReconciler(known *cluster.Cluster, model Model) Reconciler {
	return &AtomicReconciler{
		known: known,
		model: model,
	}
}

func (r *AtomicReconciler) Actual(known *cluster.Cluster) (actualCluster *cluster.Cluster, err error) {
	actualCluster = defaults.NewClusterDefaults(r.known)
	for i := 0; i < len(r.model.Resources()); i++ {
		resource := r.model.Resources()[i]
		actualCluster, _, err = resource.Actual(r.known)
		if err != nil {
			return nil, err
		}
	}
	return actualCluster, nil
}

func (r *AtomicReconciler) Expected(known *cluster.Cluster) (expectedCluster *cluster.Cluster, err error) {
	expectedCluster = defaults.NewClusterDefaults(r.known)
	for i := 0; i < len(r.model.Resources()); i++ {
		resource := r.model.Resources()[i]
		expectedCluster, _, err = resource.Expected(r.known)
		if err != nil {
			return nil, err
		}
	}
	return expectedCluster, nil
}

func (r *AtomicReconciler) cleanUp(failedCluster *cluster.Cluster, i int) (err error) {
	logger.Warning("")
	logger.Warning("Attempting to backtrack and cleanup created resources.")
	logger.Warning("")
	for j := i - 1; j >= 0; j-- {
		resource := r.model.Resources()[j]
		createdResource := createdResources[j]
		_, _, err := resource.Delete(createdResource, failedCluster)
		if err != nil {
			j, err = destroyI(err, j)
			if err != nil {
				return err
			}
			continue
		}
	}
	return nil
}

var createdResources = make(map[int]Resource)

func (r *AtomicReconciler) Reconcile(actual, expected *cluster.Cluster) (reconciledCluster *cluster.Cluster, err error) {

	reconciledCluster = defaults.NewClusterDefaults(r.known)
	for i := 0; i < len(r.model.Resources()); i++ {
		if sigHandler.GetState() != 0 {
			sigHandler.GetState()
			err := r.cleanUp(reconciledCluster, i)
			if err != nil {
				logger.Critical("Error during cleanup: %v", err)
			}
			logger.Info("Exit from reconciler.")
			os.Exit(1)
		}
		resource := r.model.Resources()[i]
		_, actualResource, err := resource.Actual(r.known)
		if err != nil {
			return nil, err
		}
		_, expectedResource, err := resource.Expected(r.known)
		if err != nil {
			return nil, err
		}
		var appliedResource Resource

		newCluster, appliedResource, err := resource.Apply(actualResource, expectedResource, reconciledCluster)
		if err != nil {
			logger.Critical("Error during apply of atomic reconciler, attempting clawback: %v", err)
			err = r.cleanUp(reconciledCluster, i)
			if err != nil {
				logger.Critical("Failure during cleanup! Abandoned resources!")
				return nil, err
			}
			return nil, nil
		}
		reconciledCluster = newCluster
		createdResources[i] = appliedResource
	}
	//teardown()
	return reconciledCluster, nil
}

var destroyRetryStrings = []string{
	"DependencyViolation:",
	"does not exist in default VPC",
}

var hg = &hang.Hanger{
	Ratio: 1,
}

func destroyI(err error, i int) (int, error) {
	hg.Hang()
	for _, retryString := range destroyRetryStrings {
		if strings.Contains(err.Error(), retryString) {
			logger.Debug("Retry failed delete: %v", err)
			time.Sleep(1 * time.Second)
			return i + 1, nil
		}
	}
	return 0, err
}

func (r *AtomicReconciler) Destroy() (destroyedCluster *cluster.Cluster, err error) {
	destroyedCluster = defaults.NewClusterDefaults(r.known)
	for i := len(r.model.Resources()) - 1; i >= 0; i-- {
		resource := r.model.Resources()[i]
		_, actualResource, err := resource.Actual(r.known)
		if err != nil {
			i, err = destroyI(err, i)
			if err != nil {
				return nil, err
			}
			continue
		}
		newCluster, _, err := resource.Delete(actualResource, destroyedCluster)
		if err != nil {
			i, err = destroyI(err, i)
			if err != nil {
				return nil, err
			}
			continue
		}
		destroyedCluster = newCluster
	}
	//teardown()
	return destroyedCluster, nil
}

func init() {
	sigHandler = signals.GlobalSignalHandler()
}

func teardown() {
	logger.Debug("Resetting TimeOut counter.")
	sigHandler.Reset()
}
