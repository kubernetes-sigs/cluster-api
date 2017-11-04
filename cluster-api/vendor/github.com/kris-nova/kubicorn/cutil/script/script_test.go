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
	"os"
	"strings"
	"testing"

	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/profiles/amazon"
)

func TestBuildBootstrapScriptHappy(t *testing.T) {
	scripts := []string{
		"bootstrap/vpn/meshbirdMaster.sh",
		"bootstrap/digitalocean_k8s_ubuntu_16.04_master.sh",
	}
	_, err := BuildBootstrapScript(scripts, &cluster.Cluster{})
	if err != nil {
		t.Fatalf("Unable to get scripts: %v", err)
	}
}

func TestBuildBootstrapScriptSad(t *testing.T) {
	scripts := []string{
		"bootstrap/vpn/meshbirdMaster.s",
		"bootstrap/digitalocean_k8s_ubuntu_16.04_master.s",
	}
	_, err := BuildBootstrapScript(scripts, &cluster.Cluster{})
	if err == nil {
		t.Fatalf("Merging non existing scripts: %v", err)
	}
}

func TestBuildBootstrapSetupScript(t *testing.T) {
	dir := "."
	fileName := "test.json"
	expectedJsonSetup := `mkdir -p .
cat <<"EOF" > ./test.json`
	expectedEnd := "\nEOF\n"

	c := amazon.NewCentosCluster("bootstrap-setup-script-test")
	os.Remove(dir + "/" + fileName)
	os.Remove("test.sh")
	script, err := buildBootstrapSetupScript(c, dir, fileName)
	if err != nil {
		t.Fatalf("Error building bootstrap setup script: %v", err)
	}
	stringScript := string(script)
	jsonCluster, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Error marshaling cluster to json: %v", err)
	}
	if shebang := "#!/usr/bin/env bash"; !strings.HasPrefix(stringScript, shebang) {
		t.Fatalf("Expected start of script is wrong!\n\nActual:\n%v\n\nExpected:\n%v", stringScript, shebang)
	}
	if !strings.HasSuffix(stringScript, expectedEnd) {
		t.Fatalf("Expected end of script is wrong!\n\nActual:\n%v\n\nExpected:\n%v", stringScript, expectedEnd)
	}
	if !strings.Contains(stringScript, expectedJsonSetup) {
		t.Fatalf("Expected script to have mkdir followed by writing to file!\n\nActual:\n%v\n\nExpected:\n%v", stringScript, expectedJsonSetup)
	}
	if !strings.Contains(stringScript, string(jsonCluster)) {
		t.Fatal("Json cluster isn't in script!")
	}
}
