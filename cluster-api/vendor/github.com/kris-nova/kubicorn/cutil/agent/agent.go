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

package agent

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/kris-nova/kubicorn/cutil/logger"
	"golang.org/x/crypto/ssh"
	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"
)

type Keyring struct {
	Agent sshagent.Agent
}

var retriveSSHKeyPassword = func() ([]byte, error) {
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return nil, fmt.Errorf("cannot detect terminal")
	}

	fmt.Print("SSH Key Passphrase [none]: ")
	passPhrase, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return nil, err
	}

	fmt.Println("")
	return passPhrase, nil
}

func NewAgent() *Keyring {
	if sysAgent := systemAgent(); sysAgent != nil {
		return &Keyring{
			Agent: sysAgent,
		}
	}

	return &Keyring{
		Agent: newKeyring(),
	}
}

func (k *Keyring) GetAgent() ssh.AuthMethod {
	return ssh.PublicKeysCallback(k.Agent.Signers)
}

func (k *Keyring) CheckKey(pubkey string) error {
	p, err := ioutil.ReadFile(pubkey)
	if err != nil {
		return err
	}

	authkey, _, _, _, _ := ssh.ParseAuthorizedKey(p)
	if err != nil {
		return err
	}
	parsedkey := authkey.Marshal()

	list, err := k.Agent.List()
	if err != nil {
		return err
	}

	for _, key := range list {
		if bytes.Equal(key.Blob, parsedkey) {
			return nil
		}
	}
	return fmt.Errorf("key not found in keyring")
}

func (k *Keyring) AddKey(pubkey string) (*Keyring, error) {
	priv, err := ioutil.ReadFile(strings.Replace(pubkey, ".pub", "", -1))
	if err != nil {
		return nil, err
	}

	key, err := privateKey(priv)
	if err != nil {
		return nil, err
	}

	newkey := sshagent.AddedKey{
		PrivateKey: key,
	}

	err = k.Agent.Add(newkey)
	if err != nil {
		return nil, err
	}

	return k, nil
}

func systemAgent() sshagent.Agent {
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		return sshagent.NewClient(sshAgent)
	}
	return nil
}

func newKeyring() sshagent.Agent {
	return sshagent.NewKeyring()
}

func privateKey(pemBytes []byte) (interface{}, error) {
	priv, err := ssh.ParseRawPrivateKey(pemBytes)
	if err != nil {
		logger.Warning(err.Error())
		passPhrase, err := retriveSSHKeyPassword()
		privwithpassphrase, err := ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, passPhrase)
		if err != nil {
			return nil, err
		}

		return privwithpassphrase, err
	}

	return priv, err
}
