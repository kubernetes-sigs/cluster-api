package simple

import (
	"fmt"
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

func (k *Kloner) Clone(repo provider.Repo) (string, error) {
	o := &git.CloneOptions{
		URL:               repo.GitCloneUrl(),
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		//Progress:          os.Stdout,
	}
	path := k.GetCloneDirectory(repo)
	local.Printf("Cloning into [%s]", path)
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
		return err
	}
	return nil
}

func (k *Kloner) GetCloneDirectory(repo provider.Repo) string {
	ws := os.Getenv("KLONE_WORKSPACE")
	if ws != "" {
		return fmt.Sprintf("%s/%s", ws, repo.Name())
	}
	wd, err := os.Getwd()
	if err != nil {
		local.RecoverableErrorf("unable to determine current directory")
		return fmt.Sprintf("%s/%s", local.Home(), repo.Name())
	}
	return fmt.Sprintf("%s/%s", wd, repo.Name())
}

func NewKloner(srv provider.GitServer) kloners.Kloner {
	return &Kloner{
		gitServer: srv,
	}
}
