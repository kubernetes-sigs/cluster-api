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
// repo.go is an implementation of a git repository according to klone

package github

import (
	"github.com/google/go-github/github"
	"github.com/kris-nova/klone/pkg/provider"
	"strings"
)

type Repo struct {
	impl         *github.Repository
	forkedFrom   *Repo
	assumedOwner string
	lang         string
}

func (r *Repo) SetImplementation(impl interface{}) {
	gh := impl.(*github.Repository)
	r.impl = gh
}

func (r *Repo) GitRemoteUrl() string {
	raw := r.impl.GetGitURL()
	replc1 := strings.Replace(raw, "://", "@", 1)
	replc2 := strings.Replace(replc1, "/", ":", 1)
	return replc2
}

func (r *Repo) GitCloneUrl() string {
	return r.impl.GetGitURL()
}

func (r *Repo) HttpsCloneUrl() string {
	return *r.impl.CloneURL
}
func (r *Repo) Language() string {
	if r.impl.Source == nil {
		if r.impl.Language != nil {
			return *r.impl.Language
		}
		return ""
	}
	if r.impl.Source.Language == nil {
		return ""
	}
	return *r.impl.Source.Language
}
func (r *Repo) Owner() string {
	if r.impl == nil {
		return r.assumedOwner
	}
	return *r.impl.Owner.Login
}
func (r *Repo) Name() string {
	return *r.impl.Name
}
func (r *Repo) Description() string {
	return *r.impl.Description
}
func (r *Repo) ForkedFrom() provider.Repo {
	if r.forkedFrom == nil {
		return nil
	}
	return r.forkedFrom
}
func (r *Repo) GetKlonefile() []byte {
	return []byte("")
}
