package klone

import (
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/klone/pkg/provider"
	"strings"
)

type QueryInformation struct {
	gitServer     provider.GitServer
	kloneProvider provider.KloneProvider
	repo          provider.Repo
	repoName      string
	repoOwner     string
}

// ParseQuery will take an arbitrary string and attempt to reason
// about what the user would like to happen for a klone. ParseQuery
// will return QueryInformation which is the information we could
// abstract out of their query and a few hits to the Git server API
func ParseQuery(query string) (bool, *QueryInformation) {
	var q *QueryInformation
	found := false
	// Check for /'s
	if strings.Contains(query, "/") {
		slashSplit := strings.Split(query, "/")
		lenSlashSplit := len(slashSplit)
		if lenSlashSplit == 1 {
			b, pqi := tryUnknown(slashSplit[0])
			if b {
				q = pqi
				found = true
			}
		} else if lenSlashSplit == 2 {
			b, pqi := tryOwnerName(slashSplit[0], slashSplit[1])
			if b {
				q = pqi
				found = true
			}
		} else if lenSlashSplit == 3 {
			b, pqi := tryServerOwnerName(slashSplit[0], slashSplit[1], slashSplit[2])
			if b {
				q = pqi
				found = true
			}
		}
	} else {
		b, pqi := tryUnknown(query)
		if b {
			q = pqi
			found = true
		}
	}
	if found {
		return true, q
	}
	return false, q
}

// tryUnknown is the most vague of all the functions. Here we
// make many assumptions, to see if we can't happen upon the repo
// the user is talking about
func tryUnknown(unknown string) (bool, *QueryInformation) {

	// First pattern is $server/$you/$repo
	ok, q := tryServerName("github.com", unknown)
	if ok {
		return true, q
	}

	// Second pattern is $name/$name like git/git or kubernetes/kubernetes
	ok, q = tryOwnerName(unknown, unknown)
	if ok {
		return true, q
	}

	return false, q
}

// tryOwnerRepo is the 2nd most granular of the the 3 try functions. This
// will try to verify a repo based on it's owner, and name. This will try
// to "guess" servers in the order they are defined in the function.
// Note: All new server implementations need to be defined here
func tryOwnerName(owner, name string) (bool, *QueryInformation) {
	// Try GitHub first
	ok, q := tryServerOwnerName("github.com", owner, name)
	if ok {
		return true, q
	}

	// Todo (@kris-nova) Define other implementations here

	return false, q
}

// tryServerName is more granular, but very useful. This function
// will try the server provider, and use the default username configured
// on your system for the owner of the repository. In the eyes of git,
// this is a fairly common occurrence.
func tryServerName(server, name string) (bool, *QueryInformation) {
	q := &QueryInformation{}
	switch server {
	case "github.com":
		ghp := NewGithubProvider()
		s, err := ghp.NewGitServer()
		if err != nil {
			local.PrintExclaimf("Unable to create new git server: %v", err)
			return false, q
		}
		repo, err := s.GetRepo(name)
		if err != nil {
			return false, q
		}
		if repo != nil {
			q.kloneProvider = ghp
			q.repo = repo
			q.repoOwner = repo.Owner()
			q.repoName = name
			q.gitServer = s
			return true, q
		}
	default:
		return false, q
	}
	return false, q
}

// tryServerOwnerRepo is the most granular of the the 3 try functions. This
// will try to verify a repo based on it's server, owner, and name. Usually
// the other functions will do a guess and check with this function.
// Note: All new server implementations need to be defined here
func tryServerOwnerName(server, owner, name string) (bool, *QueryInformation) {
	q := &QueryInformation{}
	switch server {
	case "github.com":
		ghp := NewGithubProvider()
		s, err := ghp.NewGitServer()
		if err != nil {
			local.PrintExclaimf("Unable to create new git server: %v", err)
			return false, q
		}
		repo, err := s.GetRepoByOwner(owner, name)
		if err != nil {
			return false, q
		}
		if repo != nil {
			q.kloneProvider = ghp
			q.repo = repo
			q.repoOwner = owner
			q.repoName = name
			q.gitServer = s
			return true, q
		}
	default:
		return false, q
	}

	return false, q
}
