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

package script

import (
	"encoding/json"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/parser"
)

const (
	kubicornDir             = "/etc/kubicorn"
	bootstrapInitScript     = "bootstrap_init.sh"
	clusterAsJSONFileName   = "cluster.json"
	bootstrapInitScriptBase = `#!/usr/bin/env bash

#----------------------------------------------------------
# Used only to set shebang and other environmenty things
# and is added to the beginning of other bootstrap scripts,
# including the generated "write the cluster as json" one
# to create a single user-data script for cloud providers
#----------------------------------------------------------
set -e
cd ~
`
)

func BuildBootstrapScript(bootstrapScripts []string, cluster *cluster.Cluster) ([]byte, error) {
	userData := []byte{}
	scriptData, err := buildBootstrapSetupScript(cluster, kubicornDir, clusterAsJSONFileName)
	if err != nil {
		return nil, err
	}
	userData = append(userData, scriptData...)

	for _, bootstrapScript := range bootstrapScripts {
		scriptData, err := fileresource.ReadFromResource(bootstrapScript)
		if err != nil {
			return nil, err
		}
		userData = append(userData, scriptData...)
	}

	return userData, nil
}

func buildBootstrapSetupScript(cluster *cluster.Cluster, dir, file string) ([]byte, error) {
	userData := []byte(bootstrapInitScriptBase)

	script := []byte("mkdir -p " + dir + "\ncat <<\"EOF\" > " + dir + "/" + file + "\n")

	clusterJSON, err := json.Marshal(cluster)
	if err != nil {
		return nil, err
	}

	userData = append(userData, script...)
	userData = append(userData, clusterJSON...)
	userData = append(userData, []byte("\nEOF\n")...)
	return userData, nil
}
