package cert_test

import (
	"io/ioutil"
	"os"
	"path"
	"sigs.k8s.io/cluster-api/pkg/cert"
	"testing"
)

var (
	defaultCertMaterial = "this is the cert contents"
	defaultKeyMaterial  = "this is the key contents"
)

func TestEmptyPath(t *testing.T) {
	_, err := cert.Load("")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestInvalidPath(t *testing.T) {
	_, err := cert.Load("/my/invalid/path")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestDirWithMissingKey(t *testing.T) {
	dir := newTempDir(t)
	defer os.RemoveAll(dir)
	newCaDirectory(t, dir, &defaultCertMaterial, nil)
	_, err := cert.Load(dir)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestDirWithMissingCert(t *testing.T) {
	dir := newTempDir(t)
	defer os.RemoveAll(dir)
	newCaDirectory(t, dir, nil, &defaultKeyMaterial)
	_, err := cert.Load(dir)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestDirHappyPath(t *testing.T) {
	dir := newTempDir(t)
	defer os.RemoveAll(dir)
	newCaDirectory(t, dir, &defaultCertMaterial, &defaultKeyMaterial)
	ca, err := cert.Load(dir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actualCertMaterial := string(ca.Certificate)
	if actualCertMaterial != defaultCertMaterial {
		t.Errorf("expected '%v' got '%v'", defaultCertMaterial, actualCertMaterial)
	}
	actualKeyMaterial := string(ca.PrivateKey)
	if actualKeyMaterial != defaultKeyMaterial {
		t.Errorf("expected '%v' got '%v'", defaultKeyMaterial, actualKeyMaterial)
	}
}

func TestCertPath(t *testing.T) {
	dir := newTempDir(t)
	defer os.RemoveAll(dir)
	certPath, _ := newCaDirectory(t, dir, &defaultCertMaterial, &defaultKeyMaterial)
	ca, err := cert.Load(certPath)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actualCertMaterial := string(ca.Certificate)
	if actualCertMaterial != defaultCertMaterial {
		t.Errorf("expected '%v' got '%v'", defaultCertMaterial, actualCertMaterial)
	}
	actualKeyMaterial := string(ca.PrivateKey)
	if actualKeyMaterial != defaultKeyMaterial {
		t.Errorf("expected '%v' got '%v'", defaultKeyMaterial, actualKeyMaterial)
	}
}

func TestKeyPath(t *testing.T) {
	dir := newTempDir(t)
	defer os.RemoveAll(dir)
	_, keyPath := newCaDirectory(t, dir, &defaultCertMaterial, &defaultKeyMaterial)
	ca, err := cert.Load(keyPath)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	actualCertMaterial := string(ca.Certificate)
	if actualCertMaterial != defaultCertMaterial {
		t.Errorf("expected '%v' got '%v'", defaultCertMaterial, actualCertMaterial)
	}
	actualKeyMaterial := string(ca.PrivateKey)
	if actualKeyMaterial != defaultKeyMaterial {
		t.Errorf("expected '%v' got '%v'", defaultKeyMaterial, actualKeyMaterial)
	}
}

func newCaDirectory(t *testing.T, dir string, certMaterial *string, keyMaterial *string) (certPath string, keyPath string) {
	certPath, keyPath = getCertAndKeyPaths(dir)
	if certMaterial != nil {
		err := ioutil.WriteFile(certPath, []byte(*certMaterial), 0644)
		if err != nil {
			t.Errorf("unable to write cert material to %v, got %v", certPath, err)
		}
	}
	if keyMaterial != nil {
		err := ioutil.WriteFile(keyPath, []byte(*keyMaterial), 0644)
		if err != nil {
			t.Errorf("unable to write key material to %v, got %v", keyPath, err)
		}
	}
	return
}

func getCertAndKeyPaths(dir string) (certPath string, keyPath string) {
	return path.Join(dir, "ca.crt"), path.Join(dir, "ca.key")
}

func newTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("unable to create temp dir: %v", err)
	}
	return dir
}
