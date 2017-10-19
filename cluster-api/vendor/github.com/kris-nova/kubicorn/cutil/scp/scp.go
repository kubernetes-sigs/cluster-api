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

package scp

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"
)

type SecureCopier struct {
	RemoteUser     string
	RemoteAddress  string
	RemotePort     string
	PrivateKeyPath string
}

func NewSecureCopier(remoteUser, remoteAddress, remotePort, privateKeyPath string) *SecureCopier {
	return &SecureCopier{
		RemoteUser:     remoteUser,
		RemoteAddress:  remoteAddress,
		RemotePort:     remotePort,
		PrivateKeyPath: privateKeyPath,
	}
}

func (s *SecureCopier) ReadBytes(remotePath string) ([]byte, error) {
	sshConfig := &ssh.ClientConfig{
		User:            s.RemoteUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(3 * time.Second),
	}

	agent := sshAgent()
	if agent != nil {
		sshConfig.Auth = append(sshConfig.Auth, agent)
	} else {
		pemBytes, err := ioutil.ReadFile(s.PrivateKeyPath)
		if err != nil {
			return nil, err
		}
		signer, err := getSigner(pemBytes)
		if err != nil {
			return nil, err
		}
		sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
	}

	sshConfig.SetDefaults()
	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", s.RemoteAddress, s.RemotePort), sshConfig)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c, err := sftp.NewClient(conn)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	r, err := c.Open(remotePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (s *SecureCopier) Write(localPath, remotePath string) error {
	logger.Critical("Write not yet implemented!")
	return nil
}

func getSigner(pemBytes []byte) (ssh.Signer, error) {
	signerwithoutpassphrase, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		logger.Warning(err.Error())
		fmt.Print("SSH Key Passphrase [none]: ")
		fmt.Println("")
		passPhrase, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return nil, err
		}
		signerwithpassphrase, err := ssh.ParsePrivateKeyWithPassphrase(pemBytes, passPhrase)
		if err != nil {
			return nil, err
		}

		return signerwithpassphrase, err
	}

	return signerwithoutpassphrase, err
}

func sshAgent() ssh.AuthMethod {
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
	}
	return nil
}
