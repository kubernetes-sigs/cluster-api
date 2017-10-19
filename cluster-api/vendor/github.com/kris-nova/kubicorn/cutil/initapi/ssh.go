// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package initapi

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"golang.org/x/crypto/ssh"
)

func sshLoader(initCluster *cluster.Cluster) (*cluster.Cluster, error) {
	if initCluster.SSH.PublicKeyPath != "" {
		bytes, err := ioutil.ReadFile(local.Expand(initCluster.SSH.PublicKeyPath))
		if err != nil {
			return nil, err
		}
		initCluster.SSH.PublicKeyData = bytes
		fp, err := publicKeyFingerprint(bytes)
		if err != nil {
			return nil, err
		}
		initCluster.SSH.PublicKeyFingerprint = fp
	}

	return initCluster, nil
}

func fingerprint(key ssh.PublicKey) string {
	sum := md5.Sum(key.Marshal())
	parts := make([]string, len(sum))
	for i := 0; i < len(sum); i++ {
		parts[i] = fmt.Sprintf("%0.2x", sum[i])
	}
	return strings.Join(parts, ":")
}

func publicKeyFingerprint(in []byte) (string, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey(in)
	if err != nil {
		return "", err
	}

	return fingerprint(pk), nil
}
