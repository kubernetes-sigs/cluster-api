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

package deploy

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/golang/glog"
)

const (
	MachineControllerSshKeySecret = "machine-controller-sshkeys"

	// Arbitrary name used for SSH.
	SshUser                = "clusterapi"
	SshUserFile            = "clusterapi-user"
	SshKeyFilePrivate      = "clusterapi-key"
	SshKeyFilePublic       = SshKeyFilePrivate + ".pub"
)

func createSshKeyPairs() error {
	if fileExists(SshUserFile) || fileExists(SshKeyFilePrivate) || fileExists (SshKeyFilePublic) {
		if !fileExists(SshUserFile) || !fileExists(SshKeyFilePrivate) || !fileExists (SshKeyFilePublic) {
			return fmt.Errorf(
				"an incomplete set of ssh files exist and cannot be reused. Please remove all files to regenerate ssh keys: %v, %v, %v",
				SshUserFile,
				SshKeyFilePrivate,
				SshKeyFilePublic)
		} else {
			glog.Info("Re-using existing ssh files.\n")
			return nil
		}
	}

	_, err := exec.Command("ssh-keygen", "-t", "rsa", "-f", SshKeyFilePrivate, "-C", SshUser, "-N", "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't generate RSA keys: %v", err)
	}

	f, err := os.Create(SshUserFile)
	defer f.Close()
	if err != nil {
		return fmt.Errorf("couldn't create ssh user file: %v", err)
	}

	_, err = f.WriteString(SshUser)
	if err != nil {
		return fmt.Errorf("couldn't write ssh user file: %v", err)
	}

	glog.Info("Created ssh files.\n")

	return nil
}

func cleanupSshKeyPairs() {
	os.Remove(SshKeyFilePrivate)
	os.Remove(SshKeyFilePublic)
	os.Remove(SshUserFile)
}

// It creates secret to store private key.
func setupSSHSecret() error {
	// Create secrets so that machine controller container can load them
	c := exec.Command("kubectl",
		"create",
			"secret",
			"generic",
			MachineControllerSshKeySecret,
			"--from-file=private="+SshKeyFilePrivate,
		  "--from-file=public="+SshKeyFilePublic,
			"--from-literal=user="+SshUser)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't create ssh secret. error: %v output: %v", err, string(out))
	}

	return err
}

func remoteSshCommand(ip, cmd, sshKeyPath, sshUser string) (string, error) {
	glog.Infof("Remote SSH execution '%s' on %s", cmd, ip)

	c := exec.Command("ssh", "-i", sshKeyPath, "-q", sshUser+"@"+ip, "-o", "StrictHostKeyChecking=no","-o", "UserKnownHostsFile=/dev/null", cmd)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed command '%v'. error: %v, output: %s", cmd, err, string(out))
	}
	return string(out), err
}

func sshCommand(ip, cmd string) (string, error) {
	b, err := ioutil.ReadFile(SshUserFile)
	if err != nil {
		return "", fmt.Errorf("Could not read user file %v:%v", SshUserFile, err)
	}
	user := string(b)
	return remoteSshCommand(ip, cmd, SshKeyFilePrivate, user)
}

func fileExists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}