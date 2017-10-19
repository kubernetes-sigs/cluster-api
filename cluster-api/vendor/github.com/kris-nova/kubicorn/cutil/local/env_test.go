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

package local

import (
	"os"
	"os/user"
	"testing"
)

func TestHomeWithRootHappy(t *testing.T) {
	os.Setenv("HOME", "/root")
	location := Home()
	if location != "/root" {
		t.Errorf("Home location incorrect: %s", location)
	}
}

func TestHomeAsUserHappy(t *testing.T) {
	os.Setenv("HOME", "/user/test")
	usr, _ := user.Current()
	location := Home()
	if usr.HomeDir != location {
		t.Errorf("Home location incorrect: %s should be %s", location, usr.HomeDir)
	}
}

func TestExpandAsUserHappy(t *testing.T) {
	path := Expand("~/test")
	usr, _ := user.Current()
	if path != usr.HomeDir+"/test" {
		t.Errorf("Home location incorrect: %s should be %s", path, usr.HomeDir+"/test")
	}
}

func TestExpandAsRootHappy(t *testing.T) {
	os.Setenv("HOME", "/root")
	path := Expand("~/test")
	if path != "/root/test" {
		t.Errorf("Home location incorrect: %s should be %s", path, "/root/test")
	}
}

func TestExpandAsRootNoTildeHappy(t *testing.T) {
	os.Setenv("HOME", "/root")
	path := Expand("/var/test")
	if path != "/var/test" {
		t.Errorf("Home location incorrect: %s should be %s", path, "/var/test")
	}
}

func TestExpandAsUserNoTildeHappy(t *testing.T) {
	os.Setenv("HOME", "/root")
	path := Expand("/var/test")
	if path != "/var/test" {
		t.Errorf("Home location incorrect: %s should be %s", path, "/var/test")
	}
}
