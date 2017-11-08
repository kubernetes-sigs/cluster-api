package util

import (
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"os/exec"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"os/user"
	"fmt"
	"os"
	"strings"
	"time"
	"math/rand"
)

const (
	TypeMaster = "Master"
	CharSet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func RandomToken() string {
	return fmt.Sprintf("%s.%s", randomString(6), randomString(16))
}

func randomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = CharSet[r.Intn(len(CharSet))]
	}
	return string(result)
}

func Contains (a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func IsMaster(machine *machinesv1.Machine) bool {
	return Contains(TypeMaster, machine.Spec.Roles)
}

func GetMaster(machines []machinesv1.Machine) *machinesv1.Machine {
	for _, machine := range machines {
		if IsMaster(&machine){
			return &machine
		}
	}
	return nil
}

func ExecCommand(name string, args []string) string {
	cmdOut, _ := exec.Command(name, args...).Output()
	return string(cmdOut)
}

func Home() string {
	home := os.Getenv("HOME")
	if strings.Contains(home, "root") {
		return "/root"
	}

	usr, err := user.Current()
	if err != nil {
		logger.Warning("unable to find user: %v", err)
		return ""
	}
	return usr.HomeDir
}


func GetKubeConfigPath() (string, error) {
	localDir := fmt.Sprintf("%s/.kube", Home())
	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		if err := os.Mkdir(localDir, 0777); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s/config", localDir), nil
}