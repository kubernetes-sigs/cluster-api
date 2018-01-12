/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"context"
	"flag"
)

// Stand-alone demo GCE Machines controller. Right now, it just prints simple
// information whenever it sees a create/update/delete event for any Machine,
// and creates/deletes VMs while ignoring updates.
//
// It uses the default GCP client, which expects the environment variable
// GOOGLE_APPLICATION_CREDENTIALS to point to a file with service credentials.

var kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig file")

func main() {
	flag.Parse()

	c := &controller{}
	var err error

	c.kube, _, err = restClient()
	if err != nil {
		panic(err.Error())
	}

	c.gce, err = New(c.kube)
	if err != nil {
		panic(err.Error())
	}

	c.run(context.Background())
}
