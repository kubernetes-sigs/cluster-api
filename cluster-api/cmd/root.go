package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/spf13/cobra"
	"io/ioutil"
	"k8s.io/kube-deploy/cluster-api/api"
	"k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"os"
	"os/exec"
	"crypto/md5"
	"golang.org/x/crypto/ssh"
	"strings"
)

var RootCmd = &cobra.Command{
	Use:   "cluster-api",
	Short: "cluster management",
	Long:  `Simple kubernetes cluster management`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

// Kubernetes cluster config file.
var KubeConfig string;

func init() {
	RootCmd.PersistentFlags().StringVarP(&KubeConfig, "kubecofig", "k", "", "location for the kubernetes config file")
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

func parseClusterYaml(file string) (*api.Cluster, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	cluster := &api.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}


func parseMachinesYaml(file string) ([]v1alpha1.Machine, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	machines := &v1alpha1.MachineList{}
	err = yaml.Unmarshal(bytes, &machines)
	if err != nil {
		return nil, err
	}
	return machines.Items, nil
}

// copied form github.com/kris-nova/kubicorn/cutil/initapi/ssh.go
func publicKeyFingerprint(in []byte) (string) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey(in)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	return fingerprint(pk)
}

func fingerprint(key ssh.PublicKey) string {
	sum := md5.Sum(key.Marshal())
	parts := make([]string, len(sum))
	for i := 0; i < len(sum); i++ {
		parts[i] = fmt.Sprintf("%0.2x", sum[i])
	}
	return strings.Join(parts, ":")
}
