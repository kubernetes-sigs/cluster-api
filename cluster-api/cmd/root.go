package cmd

import (
	"github.com/spf13/cobra"
	"fmt"
	"os"
	"os/exec"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

var RootCmd = &cobra.Command{
	Use:   "cluster-api",
	Short: "Simple kubernetes cluster management",
	Long: `Simple kubernetes cluster management`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

func init() {
	logger.Level = 4
}


func execCommand(name string, args []string) string {
	cmdOut, _ := exec.Command(name, args...).Output()
	return string(cmdOut)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
