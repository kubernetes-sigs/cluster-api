package externalclusterprovisioner

import (
	"testing"
	"os"
	"io/ioutil"
)

func TestGetKubeconfig(t *testing.T) {
	const contents = "dfserfafaew"
	f, err := createTempFile(contents)
	if err != nil {
		t.Fatal("Unable to create test file.")
	}
	defer os.Remove(f)

	t.Run("invalid path given", func(t *testing.T){
		_, err = NewExternalCluster("")
		if err == nil {
			t.Fatal("Should not be able create External Cluster Provisioner.")
		}
	})

	t.Run("file exists", func(t *testing.T) {

		ec, err := NewExternalCluster(f)
		if err != nil {
			t.Fatal("Should be able create External Cluster Provisioner.")
		}

		c, err := ec.GetKubeconfig()
		if err != nil {
			t.Fatalf("Unexpected err. Got: %v", err)
			return
		}
		if c != contents {
			t.Fatalf("Unexpected contents. Got: %v, Want: %v", c, contents)
		}
	})
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()
	f.WriteString(contents)
	return f.Name(), nil
}