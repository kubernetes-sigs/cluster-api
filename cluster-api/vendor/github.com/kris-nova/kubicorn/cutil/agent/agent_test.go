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

import "testing"

func TestCheckKeyWithoutPassword(t *testing.T) {
	a := NewAgent()

	err := a.CheckKey("./testdata/ssh_without_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}
}

func TestAddKeyWithoutPassword(t *testing.T) {
	a := NewAgent()

	err := a.CheckKey("./testdata/ssh_without_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}

	a, err = a.AddKey("./testdata/ssh_without_password.pub")
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}

	err = a.CheckKey("./testdata/ssh_without_password.pub")
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}
}

func TestCheckKeyWithPassword(t *testing.T) {
	a := NewAgent()

	err := a.CheckKey("./testdata/ssh_with_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}
}

func TestAddKeyWithPassword(t *testing.T) {
	retriveSSHKeyPassword = func() ([]byte, error) {
		return []byte("kubicornbesttoolever"), nil
	}
	a := NewAgent()

	err := a.CheckKey("./testdata/ssh_with_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}

	a, err = a.AddKey("./testdata/ssh_with_password.pub")
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}

	err = a.CheckKey("./testdata/ssh_with_password.pub")
	if err != nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}
}

func TestAddKeyWithPasswordIncorrect(t *testing.T) {
	retriveSSHKeyPassword = func() ([]byte, error) {
		return []byte("random"), nil
	}
	a := NewAgent()

	err := a.CheckKey("./testdata/ssh_with_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}

	a, err = a.AddKey("./testdata/ssh_with_password.pub")
	if err == nil {
		t.Fatalf("error message incorrect\n"+
			"got:       %v\n", err)
	}
}
