/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"reflect"
	"testing"
)

func TestRunCmdUnmarshal(t *testing.T) {
	cloudData := `
runcmd:
- [ ls, -l, / ]
- "ls -l /"`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	if err != nil {
		t.Fatal(err)
	}

	if len(r.Cmds) != 2 {
		t.Errorf("Expected 2 commands, found %d", len(r.Cmds))
	}

	expected0 := Cmd{Cmd: "ls", Args: []string{"-l", "/"}}
	if !reflect.DeepEqual(r.Cmds[0], expected0) {
		t.Errorf("Expected %+v commands, found %+v", expected0, r.Cmds[0])
	}

	expected1 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "ls -l /"}}
	if !reflect.DeepEqual(r.Cmds[1], expected1) {
		t.Errorf("Expected %+v commands, found %+v", expected1, r.Cmds[1])
	}
}

func TestRunCmdRun(t *testing.T) {
	var useCases = []struct {
		name         string
		r            runCmd
		expectedCmds []Cmd
	}{
		{
			name: "two command pass",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "foo", Args: []string{"bar"}},
					{Cmd: "baz", Args: []string{"bbb"}},
				},
			},
			expectedCmds: []Cmd{
				{Cmd: "foo", Args: []string{"bar"}},
				{Cmd: "baz", Args: []string{"bbb"}},
			},
		},
		{
			name: "hack kubeadm ingore errors",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config /tmp/kubeadm.yaml"}},
				},
			},
			expectedCmds: []Cmd{
				{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config /tmp/kubeadm.yaml --ignore-preflight-errors=all"}},
			},
		},
	}

	for _, rt := range useCases {
		t.Run(rt.name, func(t *testing.T) {
			commands, err := rt.r.Commands()
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(rt.expectedCmds, commands) {
				t.Errorf("expected %s, got %s", rt.expectedCmds, commands)
			}
		})
	}
}

func TestHackKubeadmIgnoreErrors(t *testing.T) {
	cloudData := `
runcmd:
- kubeadm init --config=/tmp/kubeadm.yaml
- [ kubeadm, join, --config=/tmp/kubeadm-controlplane-join-config.yaml ]`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	if err != nil {
		t.Fatal(err)
	}

	if len(r.Cmds) != 2 {
		t.Errorf("Expected 2 commands, found %d", len(r.Cmds))
	}

	r.Cmds[0] = hackKubeadmIgnoreErrors(r.Cmds[0])

	expected0 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config=/tmp/kubeadm.yaml --ignore-preflight-errors=all"}}
	if !reflect.DeepEqual(r.Cmds[0], expected0) {
		t.Errorf("Expected %+v commands, found %+v", expected0, r.Cmds[0])
	}

	r.Cmds[1] = hackKubeadmIgnoreErrors(r.Cmds[1])

	expected1 := Cmd{Cmd: "kubeadm", Args: []string{"join", "--config=/tmp/kubeadm-controlplane-join-config.yaml", "--ignore-preflight-errors=all"}}
	if !reflect.DeepEqual(r.Cmds[1], expected1) {
		t.Errorf("Expected %+v commands, found %+v", expected1, r.Cmds[1])
	}
}
