package main

import (
	"fmt"
	"os"
	goflag "flag"

	"github.com/spf13/pflag"

	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"

	"k8s.io/kube-deploy/cluster-api/machine-controller/controller"
)

func main() {
	c := controller.NewConfiguration()
	c.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	pflag.Parse()
	// Suppress warning
	// https://github.com/kubernetes/kubernetes/issues/17162
	goflag.CommandLine.Parse([]string{})
	logs.InitLogs()
	defer logs.FlushLogs()

	mc := controller.NewMachineController(c)
	if err := mc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
