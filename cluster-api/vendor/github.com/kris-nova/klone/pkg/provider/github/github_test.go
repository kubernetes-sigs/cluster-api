package github

import (
	"os"
	"testing"
)

func TestEnvUserPass(t *testing.T) {
	if os.Getenv("TEST_KLONE_GITHUBUSER") != "" {
		os.Setenv("KLONE_GITHUBUSER", os.Getenv("TEST_KLONE_GITHUBUSER"))
	}
	if os.Getenv("TEST_KLONE_GITHUBPASS") != "" {
		os.Setenv("KLONE_GITHUBPASS", os.Getenv("TEST_KLONE_GITHUBPASS"))
	}
	server := GitServer{}
	err := server.Authenticate()
	if err != nil {
		t.Fatalf("Unable to auth: %v", err)
	}
}
