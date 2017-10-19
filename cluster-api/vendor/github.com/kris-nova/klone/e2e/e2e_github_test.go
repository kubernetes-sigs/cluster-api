package e2e

import (
	"fmt"
	"github.com/kris-nova/klone/pkg/auth"
	"github.com/kris-nova/klone/pkg/klone"
	"github.com/kris-nova/klone/pkg/klone/kloners/gogit"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/klone/pkg/provider"
	"github.com/kris-nova/klone/pkg/provider/github"
	"gopkg.in/src-d/go-git.v4"
	"os"
	"strings"
	"testing"
)

var GitServer provider.GitServer

// TestMain will setup the e2e testing suite by creating a new (and concurrent) connection
// to the Git provider
func TestMain(m *testing.M) {
	os.Setenv("KLONE_WORKSPACE", local.Home()) // Always test in the home directory..
	if os.Getenv("TEST_KLONE_GITHUBTOKEN") != "" {
		os.Setenv("KLONE_GITHUBTOKEN", os.Getenv("TEST_KLONE_GITHUBTOKEN"))
	}
	if os.Getenv("TEST_KLONE_GITHUBUSER") != "" {
		os.Setenv("KLONE_GITHUBUSER", os.Getenv("TEST_KLONE_GITHUBUSER"))
	}
	if os.Getenv("TEST_KLONE_GITHUBPASS") != "" {
		os.Setenv("KLONE_GITHUBPASS", os.Getenv("TEST_KLONE_GITHUBPASS"))
	}
	github.Testing = true
	provider := klone.NewGithubProvider()
	gitServer, err := provider.NewGitServer()
	if err != nil {
		fmt.Printf("Unable to get GitHub server: %v\n", err)
		os.Exit(-1)
	}
	GitServer = gitServer
	os.Exit(m.Run())
}

// TestNewRepoOwnerKlone will ensure a throw away repository is created and then attempt to
// klone the repository.. Will ensure origin is set to the new repository
func TestNewRepoOwnerKlone(t *testing.T) {
	path := fmt.Sprintf("%s/klone-e2e-empty", local.Home())

	repo, err := GitServer.GetRepoByOwner(GitServer.OwnerName(), "klone-e2e-empty")
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("Unable to attempt to search for repo: %v", err)
	}
	if repo != nil && repo.Owner() == GitServer.OwnerName() {
		_, err := GitServer.DeleteRepo("klone-e2e-empty")
		if err != nil {
			t.Fatalf("Unable to delete repo: %v", err)
		}
	}
	repo, err = GitServer.NewRepo("klone-e2e-empty", "A throw-away repository created by Klone (@kris-nova)")
	if err != nil {
		t.Fatalf("Unable to create new repo: %v", err)
	}
	err = IdempotentKlone(path, "klone-e2e-empty")
	if err != nil {
		t.Fatalf("Error kloning: %v", err)
	}
	r, err := git.PlainOpen(path)
	if err != nil {
		t.Fatalf("Error opening path: %v", err)
	}
	remotes, err := r.Remotes()
	if err != nil {
		t.Fatalf("Error reading remotes: %v", err)
	}
	originOk := false
	for _, remote := range remotes {
		rspl := strings.Split(remote.String(), "\t")
		if len(rspl) < 3 {
			t.Fatalf("Invalid remote string: %s", remote.String())
		}
		name := rspl[0]
		url := rspl[1]
		if strings.Contains(name, "origin") && strings.Contains(url, fmt.Sprintf("git@github.com:%s/klone-e2e-empty.git", GitServer.OwnerName())) {
			originOk = true
		}
		//fmt.Println(name, url)
	}
	if originOk == false {
		t.Fatal("Error detecting remote [origin]")
	}
}

// TestGoLanguageNeedsFork will attempt to klone a repository that we KNOW the language for (Go)
// The test also handles recursively removing any local files as well as ensuring a GitHub fork
// is removed before running the test. This test (by design) will use the Golang kloner.
func TestGoLanguageNeedsFork(t *testing.T) {
	path := fmt.Sprintf("%s/src/%s/%s/klone-e2e-go", gogit.Gopath(), GitServer.GetServerString(), "Nivenly")
	t.Logf("Test path: %s", path)
	repo, err := GitServer.GetRepoByOwner(GitServer.OwnerName(), "klone-e2e-go")
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("Unable to attempt to search for repo: %v", err)
	}
	if repo != nil && repo.Owner() == GitServer.OwnerName() {
		_, err := GitServer.DeleteRepo("klone-e2e-go")
		if err != nil {
			t.Fatalf("Unable to delete repo: %v", err)
		}
	}
	err = IdempotentKlone(path, "Nivenly/klone-e2e-go")
	if err != nil {
		t.Fatalf("Error kloning: %v", err)
	}
	r, err := git.PlainOpen(path)
	if err != nil {
		t.Fatalf("Error opening path: %v", err)
	}
	remotes, err := r.Remotes()
	if err != nil {
		t.Fatalf("Error reading remotes: %v", err)
	}
	originOk, upstreamOk := false, false
	for _, remote := range remotes {
		rspl := strings.Split(remote.String(), "\t")
		if len(rspl) < 3 {
			t.Fatalf("Invalid remote string: %s", remote.String())
		}
		name := rspl[0]
		url := rspl[1]
		if strings.Contains(name, "origin") && strings.Contains(url, fmt.Sprintf("git@github.com:%s/klone-e2e-go.git", GitServer.OwnerName())) {
			originOk = true
		}
		if strings.Contains(name, "upstream") && strings.Contains(url, fmt.Sprintf("git@github.com:%s/klone-e2e-go.git", "Nivenly")) {
			upstreamOk = true
		}
	}
	if originOk == false {
		t.Fatal("Error detecting remote [origin]")
	}
	if upstreamOk == false {
		t.Fatal("Error detecting remote [upstream]")
	}
}

// TestUnknownLanguageNeedsFork will attempt to klone a repository that we KNOW we won't
// be able to detect a language for. The test also handles recursively removing any local files
// as well as ensuring a GitHub fork is removed before running the test. This test (by design)
// will use the simple kloner
func TestUnknownLanguageNeedsFork(t *testing.T) {
	path := fmt.Sprintf("%s/klone-e2e-unknown", local.Home())
	t.Logf("Test path: %s", path)
	repo, err := GitServer.GetRepoByOwner(GitServer.OwnerName(), "klone-e2e-unknown")
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("Unable to attempt to search for repo: %v", err)
	}
	if repo != nil && repo.Owner() == GitServer.OwnerName() {
		_, err := GitServer.DeleteRepo("klone-e2e-unknown")
		if err != nil {
			t.Fatalf("Unable to delete repo: %v", err)
		}
	}
	err = IdempotentKlone(path, "Nivenly/klone-e2e-unknown")
	if err != nil {
		t.Fatalf("Error kloning: %v", err)
	}
	r, err := git.PlainOpen(path)
	if err != nil {
		t.Fatalf("Error opening path: %v", err)
	}
	remotes, err := r.Remotes()
	if err != nil {
		t.Fatalf("Error reading remotes: %v", err)
	}
	originOk, upstreamOk := false, false
	for _, remote := range remotes {
		rspl := strings.Split(remote.String(), "\t")
		if len(rspl) < 3 {
			t.Fatalf("Invalid remote string: %s", remote.String())
		}
		name := rspl[0]
		url := rspl[1]
		if strings.Contains(name, "origin") && strings.Contains(url, fmt.Sprintf("git@github.com:%s/klone-e2e-unknown.git", GitServer.OwnerName())) {
			originOk = true
		}
		if strings.Contains(name, "upstream") && strings.Contains(url, fmt.Sprintf("git@github.com:%s/klone-e2e-unknown.git", "Nivenly")) {
			upstreamOk = true
		}
	}
	if originOk == false {
		t.Fatal("Error detecting remote [origin]")
	}
	if upstreamOk == false {
		t.Fatal("Error detecting remote [upstream]")
	}
}

func TestInvalidKey(t *testing.T) {
	path := fmt.Sprintf("%s/klone-e2e-unknown", local.Home())
	t.Logf("Test path: %s", path)
	repo, err := GitServer.GetRepoByOwner(GitServer.OwnerName(), "klone-e2e-unknown")
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("Unable to attempt to search for repo: %v", err)
	}
	if repo != nil && repo.Owner() == GitServer.OwnerName() {
		_, err := GitServer.DeleteRepo("klone-e2e-unknown")
		if err != nil {
			t.Fatalf("Unable to delete repo: %v", err)
		}
	}

	// Set invalid key
	auth.PrivateKeyBytesOverride = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA2v5g7tVPv/Gl+OnSwVa3Lm+Yq57y8clHy0GBbE39/aOFvAdy
68oyejwVk8XAk+KKtwOQA5PtytvpcovgT0G9IHclXxSXgIr8wnbyzgQNg6jPf2bo
0QSsPoilQTwjgTTjIdhITc5lTb0ZiJQcPICw7LKDQ4BlnWbx7zfgEuumDDse5bXW
4zdKF/TnTuKKgHYaQgL15kl5IWer8bTZAYWmhHMiDIMwXXVrabODi+aT9H/2xOPB
Y1YNFJWn9kLpMvFQix9BVHFvxsMUfDP0Va00zCoDV0SKulqx23T57ApFQuZe6cwj
L3jYKvoW+etA21EKyp539E2ITo+FbjodE2RCFQIDAQABAoIBACazmfHbZNKpJAnP
WN2uM4VTV4nM92Zfif6Tvwmi5uYyReoq7tZYz37mq3GIGzaHbLhXOtZHCFk3cBQ8
QBIBrijUpZgeDYA8D9tWJibedHz2EmWTjEWUK9SJVZsnw6aL8DAFBxIpDaIlbyPB
+ROAMsRB8Ay33j1o+gyqtUDiwF+cp1NkEB4jZA5IbXlGYYMD7bnT0mPPupO14Qwi
FmirDCUAYUlnRGzGJeZDWgJ4Z6reCj+CuFjiG1pNY5OCmZdwVgagTjb73WS+NomH
zYGkyyG8cDXo4r/E18pI2emL2Eqy5RTQoQGnbdJxcCg9j5ilkHSfli/1y0+d300K
IH83KcECgYEA+fE8Bb7wQl5AQgUSegYhjRoaizae8BVff/7Nxd6U5toWiGM8sOUL
K3fQUmVPWXvsEHQ8/yHweNEzY4/d53tGfp82LFQjhUiJGfeTa0NkCoas3M+LbHdn
PyGNzCYo9BfYWLOyUoG9xopG+r5I2vzJsykdR5pDUFrFJ7KrkAS6wbkCgYEA4E0f
mjBErtVS9GFgx3ZGkNSWrI/hCI0PiZP3VK2ViFyt48GTpGHExRX5/xzmfNdRnAOI
qEVSNiNzShUu6vNCXy8vfASihNlM9YdFn7en4fAXY2UVzgMgMeT9f63K/xCMgQYC
svcxPpauXe5SyRu6pEflpwMWCa5cLuXUt43lYT0CgYBq9+NIUj9hKBU4LBOMRbN2
QAW8LtQ2y+7jSJmoZWo0ooGiPVZsHpp5L/3oRZOAKJkPMbApQM05586mfZvqxceV
yW3V6uyIzlJe2qYs8dqO5rR0jB6PfUqiqimVhX9heBMPY1ieFwixP9zA00866ZUp
KB9v1YWZjj9yjfT+3gdPgQKBgCgo5t2egs2i2EV1IfTUknHf/lJLOIllaEDDw2bf
5YQSw/ehcyPddXOsKaSpsgLsMmpyn4cxUjTrR/sdq9bZUyrjsebMlaSfm43ieG7O
XTqw2B8tvCe0m5ETZOf1YAEnH2ECiH0ukRyZsgKTZzMmydYcIVwzy0BUfaDP+bQb
rdQpAoGBALtbN4rqe8elt0IVIEDuTk1KpTxV/gvA529dleG3iyTh/exm7SPDLcnj
3USgzH5Bnn2Z9/hgD/STKPpjufom9p73BbRBpv/pTPdpKP4fTy9WhzOrrKM0PN87
KoiaMr472EqIDviWkbWqORJi9kkm8OFw5WdirYzW8EwQFSsRjIg9
-----END RSA PRIVATE KEY-----
`)
	err = IdempotentKlone(path, "Nivenly/klone-e2e-unknown")
	if !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("unknown error while attempting invalid key: %v", err)
	}
	if err == nil {
		t.Fatal("Able to klone with invalid key")
	}
}
