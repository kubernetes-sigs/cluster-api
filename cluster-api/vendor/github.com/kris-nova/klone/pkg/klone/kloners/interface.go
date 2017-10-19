package kloners

import "github.com/kris-nova/klone/pkg/provider"

type Kloner interface {
	Clone(repo provider.Repo) (string, error)
	Pull(remote string) error
	AddRemote(name, url string) error
	DeleteRemote(name string) error
	GetCloneDirectory(repo provider.Repo) string
}
