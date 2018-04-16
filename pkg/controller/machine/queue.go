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

package machine

import (
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/controller"
	"sigs.k8s.io/cluster-api/pkg/controller/config"
)

func (c *MachineController) RunAsync(stopCh <-chan struct{}) {
	for _, q := range c.Informers.WorkerQueues {
		go c.StartWorkerQueue(q, stopCh)
	}
}

// StartWorkerQueue schedules a routine to continuously process Queue messages
// until shutdown is closed
func (c *MachineController) StartWorkerQueue(q *controller.QueueWorker, shutdown <-chan struct{}) {
	glog.Infof("Start %s Queue", q.Name)
	defer q.Queue.ShutDown()

	for i := 0; i < config.ControllerConfig.WorkerCount; i++ {
		// Every second, process all messages in the Queue until it is time to shutdown
		go wait.Until(q.ProcessAllMessages, time.Second, shutdown)
	}

	<-shutdown

	// Stop accepting messages into the Queue
	glog.V(1).Infof("Shutting down %s Queue\n", q.Name)
}
