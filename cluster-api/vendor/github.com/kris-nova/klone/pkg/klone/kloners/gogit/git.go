package gogit

import (
	"fmt"
	//"github.com/kris-nova/klone/pkg/auth"
	"github.com/kris-nova/klone/pkg/auth"
	"github.com/kris-nova/klone/pkg/klone/kloners"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/klone/pkg/provider"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"os"
	"strings"
)

type Kloner struct {
	gitServer provider.GitServer
	r         *git.Repository
}

// This is the logic that defins a Clone() for a Go repository
// Of course we need to check out into $GOPATH
func (k *Kloner) Clone(repo provider.Repo) (string, error) {
	o := &git.CloneOptions{
		URL:               repo.GitCloneUrl(),
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		//Progress:          os.Stdout,
	}
	path := k.GetCloneDirectory(repo)
	local.Printf("Cloning into $GOPATH [%s]", path)
	r, err := git.PlainClone(path, false, o)
	if err != nil {
		if strings.Contains(err.Error(), "repository already exists") {
			local.Printf("Clone: %s", err.Error())
			r, err := git.PlainOpen(path)
			if err != nil {
				return "", err
			}
			k.r = r
			return path, nil
		} else if strings.Contains(err.Error(), "unknown capability") {
			// Todo (@kris-nova) handle capability errors better https://github.com/kris-nova/klone/issues/5
			local.RecoverableErrorf("bypassing capability error: %v ", err)
		} else {
			return "", fmt.Errorf("unable to clone repository: %v", err)
		}
	}
	local.Printf("Checking out HEAD")
	ref, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("unable to checkout HEAD: %v", err)
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("unable to checkout latest commit: %v", err)
	}
	local.Printf("HEAD checked out HEAD at [%s]", commit.Hash)

	// Update our repository for the other commands
	k.r = r
	return path, nil
}

func (k *Kloner) DeleteRemote(name string) error {
	err := k.r.DeleteRemote(name)
	if err != nil {
		return err
	}
	return nil
}

// Add remote will add a new remote, and fetch from the remote branch
func (k *Kloner) AddRemote(name, url string) error {
	c := &config.RemoteConfig{
		Name: name,
		URL:  url,
	}
	local.Printf("Adding remote [%s][%s]", name, url)
	r, err := k.r.CreateRemote(c)
	if err != nil {
		if strings.Contains(err.Error(), "remote already exists") {
			local.Printf("Remote: %s", err.Error())
			return nil
		} else {
			return fmt.Errorf("unable create remote: %v", err)
		}
	}

	local.Printf("Fetching remote [%s]", url)
	pk, err := auth.GetTransport()
	if err != nil {
		return err
	}
	f := &git.FetchOptions{
		RemoteName: name,
		Auth:       pk,
		//Progress:   os.Stdout,
	}
	err = r.Fetch(f)
	if err != nil {
		if strings.Contains(err.Error(), "already up-to-date") {
			local.Printf("Fetch: %s", err.Error())
			return nil
		}
		return fmt.Errorf("unable to fetch remote: %v", err)
	}
	return nil
}

func (k *Kloner) Pull(name string) error {
	pk, err := auth.GetTransport()
	if err != nil {
		return err
	}
	o := &git.PullOptions{
		RemoteName: name,
		//Progress:   os.Stdout,
		Auth: pk,
	}
	local.Printf("Pulling remote [%s]", name)
	err = k.r.Pull(o)
	if err != nil {
		if strings.Contains(err.Error(), "already up-to-date") {
			local.Printf("Pull: %s", err.Error())
			return nil
		}
		return fmt.Errorf("unable to pull remote: %v", err)
	}
	return nil
}

func NewKloner(srv provider.GitServer) kloners.Kloner {
	return &Kloner{
		gitServer: srv,
	}
}

type customPathFunc func(repo provider.Repo) string

// customPaths is a map of git owners to conversion functions
var customPaths = map[string]customPathFunc{
	"kubernetes": repoToKubernetesPath,
}

// repoToCloneDirectory will take a repository and reason about
// where to check out the repository on your local filesystem
func (k *Kloner) GetCloneDirectory(repo provider.Repo) string {
	var path string
	// Default path
	path = fmt.Sprintf("%s/src/%s/%s/%s", Gopath(), k.gitServer.GetServerString(), repo.Owner(), repo.Name())
	// Check for custom path overrides
	for owner, customFunc := range customPaths {
		if repo.Owner() == owner {
			path = customFunc(repo)
			//local.Printf("Local path override to [%s]", path)
			break
		}
	}
	return path
}

// Logic for getting $GOPATH
func Gopath() string {
	epath := os.Getenv("GOPATH")
	if epath == "" {
		// It's now safe to assume $HOME/go
		// thanks to Dave Cheney and the folks
		// who work on the standard library
		// https://github.com/golang/go/issues/17262
		path := fmt.Sprintf("%s/go", local.Home())
		return path
	} else if strings.Contains(epath, ":") {
		espl := strings.Split(epath, ":")
		if len(espl) <= 1 {
			return epath
		}
		// Here we will take the first gopath defined and use that
		return espl[0]
	}
	return epath
}
