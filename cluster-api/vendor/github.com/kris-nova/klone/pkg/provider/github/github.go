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
// server.go is a representation of GitHub.com as a git server

package github

import (
	"bufio"
	"context"
	"fmt"
	"github.com/google/go-github/github"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/klone/pkg/provider"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/oauth2"
	"os"
	"strings"
	"syscall"
)

var (
	Cache              = fmt.Sprintf("%s/.klone/auth", local.Home())
	RefreshCredentials = false
	Testing            = false
)

const (
	AccessTokenNote = "Access token automatically managed my Klone. More information: https://github.com/kris-nova/klone."
)

// GitServer is a representation of GitHub.com, by design we never store credentials here in memory
// Todo (@kris-nova) encrypt these creds please!
type GitServer struct {
	username string
	client   *github.Client
	ctx      context.Context
	cursor   provider.Repo
	repos    map[string]provider.Repo
	usr      *github.User
}

func (s *GitServer) Fork(parent provider.Repo, newOwner string) (provider.Repo, error) {
	c := &github.RepositoryCreateForkOptions{}
	// Override c.Orginzation here if we ever need one!
	repo, _, err := s.client.Repositories.CreateFork(s.ctx, parent.Owner(), parent.Name(), c)
	if err != nil {
		return nil, fmt.Errorf("unable to fork repository [%s]: %v", parent.Name(), err)
	}
	r := &Repo{
		impl: repo,
	}
	return r, nil
}

// GetServerString returns a server string is the string we would want to use in things like $GOPATH
// In this case we know we are dealing with GitHub.com so we can safely return it.
func (s *GitServer) GetServerString() string {
	return "github.com"
}

func (s *GitServer) OwnerName() string {
	return *s.usr.Login
}
func (s *GitServer) OwnerEmail() string {
	return *s.usr.Email
}

// Authenticate will parse configuration with the following hierarchy.
// 1. Access token from local cache
// 2. Access token from env var
// 3. Username/Password from env var
// Authenticate will then attempt to log in (prompting for MFA if necessary)
// Authenticate will then attempt to ensure a unique access token created by klone for future access
// To ensure a new auth token, simply set the env var and klone will re-cache the new token
func (s *GitServer) Authenticate() error {
	credentials, err := s.getCredentials()
	if err != nil {
		return err
	}
	r := bufio.NewReader(os.Stdin)
	token := credentials.Token
	s.ctx = context.Background()
	var client *github.Client
	var tp github.BasicAuthTransport
	if credentials.Token != "" && !RefreshCredentials {
		local.Printf("Auth [token]")
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(s.ctx, ts)
		client = github.NewClient(tc)
	} else {
		local.Printf("Auth [user/pass]")
		username := credentials.User
		password := credentials.Pass
		tp = github.BasicAuthTransport{
			Username: strings.TrimSpace(username),
			Password: strings.TrimSpace(password),
		}
		client = github.NewClient(tp.Client())
	}
	s.client = client
	user, _, err := client.Users.Get(s.ctx, "")
	s.usr = user
	// Check if we need 2 factor
	if _, ok := err.(*github.TwoFactorAuthError); ok {
		local.PrintPrompt("GitHub Two Factor Auth Code: ")
		mfa, _ := r.ReadString('\n')
		tp.OTP = strings.TrimSpace(mfa)
		user, _, err := client.Users.Get(s.ctx, "")
		if err != nil {
			return err
		}
		s.usr = user
	} else if err != nil {
		return err
	}
	if RefreshCredentials {
		s.refreshToken()
	}
	name := *s.usr.Login
	s.username = name
	local.Printf("Successfully authenticated [%s]", name)
	//s.ensureLocalAuthToken(token)
	return nil
}

// GetRepoByOwner is the most effecient way to look up a repository exactly by it's name and owner
func (s *GitServer) GetRepoByOwner(owner, name string) (provider.Repo, error) {
	r := &Repo{}
	repo, _, err := s.client.Repositories.Get(s.ctx, owner, name)
	if err != nil {
		local.Printf("Unable to find repo [%s/%s]", owner, name)
		return r, err
	}
	if repo == nil {
		return r, nil
	}
	r.impl = repo
	r.assumedOwner = owner
	if *repo.Fork && repo.Parent.Owner != nil {
		r.forkedFrom = &Repo{impl: repo.Parent}
	}
	return r, nil

}

func (s *GitServer) NewRepo(name, desc string) (provider.Repo, error) {
	t := true
	gitRepo := &github.Repository{}
	gitRepo.Name = &name
	gitRepo.Description = &desc
	gitRepo.AutoInit = &t
	repo, _, err := s.client.Repositories.Create(s.ctx, "", gitRepo)
	if err != nil {
		return nil, err
	}
	r := &Repo{}
	r.impl = repo

	return r, nil
}

func (s *GitServer) DeleteRepoByOwner(name, owner string) (bool, error) {
	_, err := s.client.Repositories.Delete(s.ctx, owner, name)
	if err != nil {
		return false, err
	}
	return true, nil

}

func (s *GitServer) DeleteRepo(name string) (bool, error) {
	_, err := s.client.Repositories.Delete(s.ctx, s.username, name)
	if err != nil {
		return false, err
	}
	return true, nil

}

// GetRepo is the most effecient way to look up a repository exactly by it's name and assumed owner (you)
func (s *GitServer) GetRepo(name string) (provider.Repo, error) {
	r := &Repo{}
	repo, _, err := s.client.Repositories.Get(s.ctx, s.username, name)
	if err != nil {
		local.Printf("Unable to find repo [%s/%s]", s.username, name)
		return r, err
	}
	if repo == nil {
		return r, nil
	}
	r.impl = repo
	if *repo.Fork {
		r.forkedFrom = &Repo{impl: repo.Parent}
	}
	return r, nil
}

// GetRepos will return (and cache) a hash map of repositories by name for some
// convenient O(n*log(n)) look up!
func (s *GitServer) GetRepos() (map[string]provider.Repo, error) {
	providerRepos := make(map[string]provider.Repo)
	if len(s.repos) == 0 {
		opt := &github.RepositoryListOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			repos, resp, err := s.client.Repositories.List(s.ctx, s.username, opt)
			if err != nil {
				return providerRepos, err
			}
			for _, repo := range repos {
				r := &Repo{impl: repo}
				providerRepos[*r.impl.Name] = r
				// Here is where we look up forked repo information (if we have it)
				if *repo.Fork {
					rr, _, err := s.client.Repositories.Get(s.ctx, s.username, *r.impl.Name)
					if err != nil {
						return providerRepos, err
					}
					parent := &Repo{impl: rr.Parent}
					r.forkedFrom = parent
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opt.ListOptions.Page = resp.NextPage
		}
		s.repos = providerRepos
	}
	local.Printf("Cached %d repositories in memory", len(s.repos))
	return s.repos, nil
}

func (s *GitServer) refreshToken() {
	req := github.AuthorizationRequest{}
	note := AccessTokenNote
	req.Note = &note
	var scopes []github.Scope
	scopes = append(scopes, github.ScopeDeleteRepo)
	scopes = append(scopes, github.ScopeRepo)
	scopes = append(scopes, github.ScopeUser)
	req.Scopes = scopes
	var auth *github.Authorization
	auth, resp, err := s.client.Authorizations.Create(s.ctx, &req)
	if resp.StatusCode == 422 && strings.Contains(err.Error(), "Code:already_exists") {
		// It already exists let's delete and create a new one
		auths, _, err := s.client.Authorizations.List(s.ctx, nil)
		if err != nil {
			local.RecoverableErrorf("Unable to delete existing auth token [%s]: %v", *auth.ID, err)
			return
		}
		for _, a := range auths {
			n := *a.Note
			if n == AccessTokenNote {
				_, err := s.client.Authorizations.Delete(s.ctx, *a.ID)
				if err != nil {
					local.RecoverableErrorf("Unable to delete existing auth token [%s]: %v", *auth.ID, err)
					return
				}
				auth, _, err = s.client.Authorizations.Create(s.ctx, &req)
				if err != nil {
					local.RecoverableErrorf("Unable to create new auth token [%s]: %v", *auth.ID, err)
					return
				}
				break
			}
		}
	} else if err != nil {
		local.RecoverableErrorf("Unable to generate new auth token: %v", err)
		return
	}
	str := *auth.Token
	err = local.SPutContent(str, Cache)
	if err != nil {
		local.RecoverableErrorf("Unable to ensure local auth token: %v", err)
		return
	}
	os.Setenv("KLONE_GITHUBTOKEN", str)
	RefreshCredentials = false
	local.Printf("Successfully cached access token!")
}

// GitHubCredentials are how we log into GitHub.com
type GitHubCredentials struct {
	User  string
	Pass  string
	Token string
}

var creds *GitHubCredentials

func (s *GitServer) getCredentials() (*GitHubCredentials, error) {
	if creds != nil {
		return creds, nil
	}
	creds = &GitHubCredentials{}
	var token string
	var user string
	var pass string

	setToken := os.Getenv("KLONE_GITHUBTOKEN")
	cachedToken := local.SGetContent(Cache)

	// We have a token in memory, this always wins
	if !RefreshCredentials {
		if setToken != "" {
			token = setToken
			if setToken != cachedToken {
				// Override cache
				local.RecoverableError("Conflicting tokens, default to token in memory.")
				os.MkdirAll(fmt.Sprintf("%s/.klone", local.Home()), 0700)
				local.SPutContent(token, Cache)
			}

		} else if cachedToken != "" {
			os.Setenv("KLONE_GITHUBTOKEN", cachedToken)
			token = cachedToken
		} else if !Testing {
			// We need a new token
			RefreshCredentials = true
		}
	}

	if token == "" {
		r := bufio.NewReader(os.Stdin)
		user = os.Getenv("KLONE_GITHUBUSER")
		if user == "" || RefreshCredentials {
			local.PrintPrompt("GitHub Username: ")
			u, err := r.ReadString('\n')
			if err != nil {
				return creds, err
			}
			user = strings.TrimSpace(u)
		}
		pass = os.Getenv("KLONE_GITHUBPASS")
		if pass == "" || RefreshCredentials {
			local.PrintPrompt("GitHub Password: ")
			bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
			p := string(bytePassword)
			pass = strings.TrimSpace(p)
		}
	}

	creds.Token = token
	creds.User = user
	creds.Pass = pass

	return creds, nil
}
