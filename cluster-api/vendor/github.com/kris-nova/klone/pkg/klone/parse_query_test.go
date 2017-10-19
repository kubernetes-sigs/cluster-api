package klone

import (
	"fmt"
	"github.com/kris-nova/klone/pkg/provider"
	"os"
	"strings"
	"testing"
)

// Repositories we know we can test with
// github.com/Nivenly/klone-e2e-go
// github.com/Nivenly/klone-e2e-unknown
// github.com/$you/klone-e2e-query

var GitServer provider.GitServer

// TestMain will setup the e2e testing suite by creating a new (and concurrent) connection
// to the Git provider
func TestMain(m *testing.M) {
	provider := NewGithubProvider()
	gitServer, err := provider.NewGitServer()
	if err != nil {
		fmt.Printf("Unable to get GitHub server: %v\n", err)
		os.Exit(-1)
	}
	GitServer = gitServer

	// Idempotent create test repo
	repo, err := GitServer.GetRepoByOwner(GitServer.OwnerName(), "klone-e2e-query")
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		fmt.Printf("Unable to attempt to search for repo: %v\n", err)
		os.Exit(-1)
	}
	if repo != nil && repo.Owner() == GitServer.OwnerName() {
		_, err := GitServer.DeleteRepo("klone-e2e-query")
		if err != nil {
			fmt.Printf("Unable to delete repo: %v\n", err)
			os.Exit(-1)
		}
	}
	repo, err = gitServer.NewRepo("klone-e2e-query", "A throw-away repository created by Klone (@kris-nova)")
	if err != nil {
		fmt.Printf("Unable to create test repository: %v", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}

// TestParseQueryTripleGithub will test a well known org repo that is not
// owned by the user
func TestWellKnownOrgRepo(t *testing.T) {
	b, q := ParseQuery("Nivenly/klone-e2e-go")
	if !b {
		t.Fatal("Unable to parse well known org query")
	}
	if q.gitServer.GetServerString() != "github.com" {
		t.Fatal("Unable to detect GitHub for well known org query")
	}
	if q.repoName != "klone-e2e-go" {
		t.Fatal("Unable to detect repo name for well known org query")
	}
	if q.repoOwner != "Nivenly" {
		t.Fatal("Unable to match owner for well known org query")
	}
}

// TestParseQuerySingleGithub will test that the single query "klone-e2e-query" will return
// the proper QueryInformation
func TestParseQuerySingleGithub(t *testing.T) {
	b, q := ParseQuery("klone-e2e-query")
	if !b {
		t.Fatal("Unable to parse single query")
	}
	if q.gitServer.GetServerString() != "github.com" {
		t.Fatal("Unable to detect GitHub for single query")
	}
	if q.repoName != "klone-e2e-query" {
		t.Fatal("Unable to detect repo name for single query")
	}
	if q.repoOwner != GitServer.OwnerName() {
		t.Fatal("Unable to match owner for single query")
	}
}

// TestParseQueryDoubleGithub will test that the single query "klone-e2e-query" will return
// the proper QueryInformation
func TestParseQueryDoubleGithub(t *testing.T) {
	b, q := ParseQuery("kris-nova/klone-e2e-query")
	if !b {
		t.Fatal("Unable to parse double query")
	}
	if q.gitServer.GetServerString() != "github.com" {
		t.Fatal("Unable to detect GitHub for double query")
	}
	if q.repoName != "klone-e2e-query" {
		t.Fatal("Unable to detect repo name for double query")
	}
	if q.repoOwner != "kris-nova" {
		t.Fatal("Unable to match owner for double query")
	}
}

// TestParseQueryTripleGithub will test that the single query "klone-e2e-query" will return
// the proper QueryInformation
func TestParseQueryTripleGithub(t *testing.T) {
	b, q := ParseQuery("github.com/kris-nova/klone-e2e-query")
	if !b {
		t.Fatal("Unable to parse triple query")
	}
	if q.gitServer.GetServerString() != "github.com" {
		t.Fatal("Unable to detect GitHub for triple query")
	}
	if q.repoName != "klone-e2e-query" {
		t.Fatal("Unable to detect repo name for triple query")
	}
	if q.repoOwner != "kris-nova" {
		t.Fatal("Unable to match owner for triple query")
	}
}
