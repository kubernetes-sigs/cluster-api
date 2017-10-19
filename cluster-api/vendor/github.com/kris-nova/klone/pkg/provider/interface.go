// Copyright © 2017 Kris Nova <kris@nivenly.com>
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
//
//  _  ___
// | |/ / | ___  _ __   ___
// | ' /| |/ _ \| '_ \ / _ \
// | . \| | (_) | | | |  __/
// |_|\_\_|\___/|_| |_|\___|
//
// interface.go defines the kloneprovider interfaces. These represent the logic we will need
// to work with the klone (and other) routines.

package provider

// KloneProvider is the core provider for using Klone
type KloneProvider interface {
	NewGitServer() (GitServer, error)
}

// Command represents a single task to perform in with git
type Command interface {
	Exec()
	GetStdErr() ([]byte, error)
	GetStdOut() ([]byte, error)
	Next() Command
}

// Repo represents a git repository
type Repo interface {
	GitCloneUrl() string
	GitRemoteUrl() string
	HttpsCloneUrl() string
	Language() string
	Owner() string
	Name() string
	Description() string
	ForkedFrom() Repo
	GetKlonefile() []byte
	SetImplementation(interface{})
}

// GitServer represents a git server (like github.com)
type GitServer interface {
	Authenticate() error
	GetServerString() string
	GetRepos() (map[string]Repo, error)
	GetRepo(string) (Repo, error)
	GetRepoByOwner(owner, name string) (Repo, error)
	OwnerName() string
	OwnerEmail() string
	Fork(Repo, string) (Repo, error)
	DeleteRepo(string) (bool, error)
	DeleteRepoByOwner(owner, name string) (bool, error)
	NewRepo(name, desc string) (Repo, error)
}
