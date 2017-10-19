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

package initapi

import (
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

type preProcessorFunc func(initCluster *cluster.Cluster) (*cluster.Cluster, error)

var preProcessors = []preProcessorFunc{
	sshLoader,
}

type validationFunc func(initCluster *cluster.Cluster) error

var validations = []validationFunc{
	validateAtLeastOneServerPool,
	validateServerPoolMaxCountGreaterThan1,
	validateSpotPriceOnlyForAwsCluster,
}

func InitCluster(initCluster *cluster.Cluster) (*cluster.Cluster, error) {
	logger.Info("Init Cluster")
	logger.Debug("Running preprocessors")
	for _, f := range preProcessors {
		var err error
		initCluster, err = f(initCluster)
		if err != nil {
			return nil, err
		}
	}
	logger.Debug("Running validations")
	for _, f := range validations {
		var err error
		err = f(initCluster)
		if err != nil {
			return nil, err
		}
	}
	return initCluster, nil
}
