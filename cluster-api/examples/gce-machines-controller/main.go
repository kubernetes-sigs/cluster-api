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
