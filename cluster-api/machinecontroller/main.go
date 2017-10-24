package main

import (
	"fmt"
	"os"
	"github.com/spf13/pflag"
	"k8s.io/kube-deploy/cluster-api/machinecontroller/controller"

)

func main() {
	c := controller.NewConfiguration()
	c.AddFlags(pflag.CommandLine)

	// Setup logging

	if err := controller.Run(c); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}