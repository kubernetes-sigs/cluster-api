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
	"testing"

	. "github.com/onsi/gomega"
)

func TestRunCmdUnmarshal(t *testing.T) {
	g := NewWithT(t)

	cloudData := `
runcmd:
- [ ls, -l, / ]
- "ls -l /"`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(r.Cmds).To(HaveLen(2))

	expected0 := Cmd{Cmd: "ls", Args: []string{"-l", "/"}}
	g.Expect(r.Cmds[0]).To(Equal(expected0))

	expected1 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "ls -l /"}}
	g.Expect(r.Cmds[1]).To(Equal(expected1))
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
			g := NewWithT(t)

			commands, err := rt.r.Commands()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rt.expectedCmds).To(Equal(commands))
		})
	}
}

func TestHackKubeadmIgnoreErrors(t *testing.T) {
	g := NewWithT(t)

	cloudData := `
runcmd:
- kubeadm init --config=/tmp/kubeadm.yaml
- [ kubeadm, join, --config=/tmp/kubeadm-controlplane-join-config.yaml ]`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(r.Cmds).To(HaveLen(2))

	r.Cmds[0] = hackKubeadmIgnoreErrors(r.Cmds[0])

	expected0 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config=/tmp/kubeadm.yaml --ignore-preflight-errors=all"}}
	g.Expect(r.Cmds[0]).To(Equal(expected0))

	r.Cmds[1] = hackKubeadmIgnoreErrors(r.Cmds[1])

	expected1 := Cmd{Cmd: "kubeadm", Args: []string{"join", "--config=/tmp/kubeadm-controlplane-join-config.yaml", "--ignore-preflight-errors=all"}}
	g.Expect(r.Cmds[1]).To(Equal(expected1))
}
