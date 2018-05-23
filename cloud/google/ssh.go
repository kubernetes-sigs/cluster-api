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

package google

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func (gce *GCEClient) sshMetadata(metadata map[string]string) (map[string]string, error) {
	sshMetadata(metadata, gce.sshCreds.user, gce.sshCreds.publicKey)
	return metadata, nil
}

func sshMetadata(metadata map[string]string, user, publicKey string) (map[string]string) {
	metadata["ssh-keys"] = fmt.Sprintf("%s:%s", user, publicKey)
	return metadata
}

func (gce *GCEClient) remoteSshCommand(m *clusterv1.Machine, cmd string) (string, error) {
	publicIP, err := gce.GetIP(m)
	if err != nil {
		return "", err
	}

	glog.Infof("Remote SSH execution '%s' on %s %s", cmd, m.ObjectMeta.Name, publicIP)
  return remoteSshCommand(publicIP, cmd, gce.sshCreds.user, gce.sshCreds.privateKeyPath)
}

func remoteSshCommand(ip, cmd, user, privateKeyPath string) (string, error) {
	out, err := runOutput("ssh", "-i", privateKeyPath, "-q", user+"@"+ip, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", cmd)
	if err != nil {
		return "", fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	result := strings.TrimSpace(string(out))
	return result, nil
}