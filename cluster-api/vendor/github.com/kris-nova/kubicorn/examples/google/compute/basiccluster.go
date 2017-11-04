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

package main

import (
	"github.com/kris-nova/kubicorn/cutil"
	"github.com/kris-nova/kubicorn/cutil/initapi"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/profiles/googlecompute"
)

func main() {
	logger.Level = 4
	cluster := googlecompute.NewUbuntuCluster("myCluster")
	cluster, err := initapi.InitCluster(cluster)
	if err != nil {
		panic(err.Error())
	}
	reconciler, err := cutil.GetReconciler(cluster, nil)
	if err != nil {
		panic(err.Error())
	}
	expected, err := reconciler.Expected(cluster)
	if err != nil {
		panic(err.Error())
	}
	actual, err := reconciler.Actual(cluster)
	if err != nil {
		panic(err.Error())
	}
	created, err := reconciler.Reconcile(actual, expected)
	logger.Info("Created cluster [%s]", created.Name)
	if err != nil {
		panic(err.Error())
	}
	_, err = reconciler.Destroy()
	if err != nil {
		panic(err.Error())
	}
}
