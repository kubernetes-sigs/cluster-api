package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"

	"k8s.io/kube-deploy/cluster-api/machinecontroller/controller"
)

func main() {
	c := controller.NewConfiguration()
	c.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	mc := controller.NewMachineController(c)
	if err := mc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
