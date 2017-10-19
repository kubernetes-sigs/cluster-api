package klone

import (
	"fmt"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/klone/pkg/provider"
	"strings"
	"time"
)

const waitForForkSeconds = 10

// kloneOwner is the function that is called when we will be cloning into
// a repository where we are the owner. E.G. this is ours, and not a fork.
func (k *Kloneable) kloneOwner() (string, error) {
	local.Printf("Attempting git clone")
	path, err := k.kloner.Clone(k.repo)
	if err != nil {
		return "", err
	}
	// Always delete origins
	local.Printf("Register remote [origin]")
	err = k.kloner.DeleteRemote("origin")
	if err != nil && !strings.Contains(err.Error(), "remote not found") {
		return path, err
	}
	// Add Origin
	// Origin is our remote URL, and location is ours too!
	err = k.kloner.AddRemote("origin", k.repo.GitRemoteUrl())
	if err != nil {
		return path, err
	}
	return path, nil
}

// kloneAlreadyForked is called when we are cloning into a parent repository
// on disk, but the origin is ours.
func (k *Kloneable) kloneAlreadyForked() (string, error) {
	local.Printf("Attempting git clone")
	path, err := k.kloner.Clone(k.repo.ForkedFrom())
	if err != nil {
		return "", err
	}
	// Add Origin
	local.Printf("Register remote [origin]")
	err = k.kloner.DeleteRemote("origin")
	if err != nil && !strings.Contains(err.Error(), "remote not found") {
		return path, err
	}
	// Origin is our remote URL, but their location on disk
	err = k.kloner.AddRemote("origin", k.repo.GitRemoteUrl())
	if err != nil {
		return path, err
	}
	local.Printf("Register remote [upstream]")
	err = k.kloner.DeleteRemote("upstream")
	if err != nil && !strings.Contains(err.Error(), "remote not found") {
		return path, err
	}
	// Upstream is their remote URL, and their location on disk
	err = k.kloner.AddRemote("upstream", k.repo.ForkedFrom().GitRemoteUrl())
	if err != nil {
		return path, err
	}
	// Pull
	local.Printf("Pull [upstream]")
	err = k.kloner.Pull("upstream")
	if err != nil {
		return path, err
	}
	return path, nil
}

// kloneTryingFork wraps kloneNeedsFork()
func (k *Kloneable) kloneTryingFork() (string, error) {
	return k.kloneNeedsFork()
}

// kloneNeedsFork will first attempt to fork a repository before cloning the repository.
// We will clone to the parent's location on disk, but with our origin
func (k *Kloneable) kloneNeedsFork() (string, error) {
	local.Printf("Forking [%s/%s] to [%s/%s]", k.repo.Owner(), k.repo.Name(), k.gitServer.OwnerName(), k.repo.Name())
	var newRepo provider.Repo
	newRepo, err := k.gitServer.Fork(k.repo, k.gitServer.OwnerName())
	if err != nil {
		if strings.Contains(err.Error(), "job scheduled on GitHub side") || strings.Contains(err.Error(), "404 Not Found") {
			// Forking might take a while, so poll for it
			for i := 1; i <= waitForForkSeconds; i++ {
				repo, err := k.gitServer.GetRepo(k.repo.Name())
				newRepo = repo
				if err == nil {
					local.Printf("Succesfully detected new repository [%s/%s]", repo.Owner(), repo.Name())
					break
				}
				if i == waitForForkSeconds {
					return "", fmt.Errorf("unable to detect forked repository after waiting %d seconds", waitForForkSeconds)
				}
				time.Sleep(time.Second * 1)
			}
		} else {
			return "", err
		}
	}
	local.Printf("Attempting git clone")
	// clone with the original repo
	path, err := k.kloner.Clone(k.repo)
	if err != nil {
		return "", err
	}
	// Always delete origin
	local.Printf("Register remote [origin]")
	err = k.kloner.DeleteRemote("origin")
	if err != nil && !strings.Contains(err.Error(), "remote not found") {
		return path, err
	}
	// Add origin
	err = k.kloner.AddRemote("origin", newRepo.GitRemoteUrl())
	if err != nil {
		return path, err
	}

	// Add Upstream
	local.Printf("Register remote [upstream]")
	err = k.kloner.DeleteRemote("upstream")
	if err != nil && !strings.Contains(err.Error(), "remote not found") {
		return path, err
	}
	err = k.kloner.AddRemote("upstream", k.repo.GitRemoteUrl())
	if err != nil {
		return path, err
	}

	// Pull
	err = k.kloner.Pull("upstream")
	if err != nil {
		return path, err
	}

	return path, nil
}
