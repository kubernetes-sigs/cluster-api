/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package google

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/golang/glog"
)

var machineControllerImage = "gcr.io/k8s-cluster-api/apiserver-controller:0.1"

func init() {
	if img, ok := os.LookupEnv("MACHINE_CONTROLLER_IMAGE"); ok {
		machineControllerImage = img
	}
}

type caCertParams struct {
	caBundle string
	tlsCrt   string
	tlsKey   string
}

func getBase64(file string) string {
	buff := bytes.Buffer{}
	enc := base64.NewEncoder(base64.StdEncoding, &buff)
	data, err := ioutil.ReadFile(file)
	if err != nil {
		glog.Fatalf("Could not read file %s: %v", file, err)
	}

	_, err = enc.Write(data)
	if err != nil {
		glog.Fatalf("Could not write bytes: %v", err)
	}
	enc.Close()
	return buff.String()
}

func getApiServerCerts() (*caCertParams, error) {
	// TODO: remove the need to use apiserver-boot to generate the certificate.
	os.RemoveAll("./config")
	err := run("apiserver-boot", "build", "config",
		"--name", "clusterapi", "--namespace", "default",
		"--image", machineControllerImage)
	if err != nil {
		return nil, err
	}

	certParms := &caCertParams{
		caBundle: getBase64("config/certificates/apiserver_ca.crt"),
		tlsCrt:   getBase64("config/certificates/apiserver.crt"),
		tlsKey:   getBase64("config/certificates/apiserver.key"),
	}

	return certParms, nil
}

func CreateApiServerAndController(token string) error {
	tmpl, err := template.ParseFiles("cloud/google/config/apiserver.yaml")
	if err != nil {
		return err
	}

	certParms, err := getApiServerCerts()
	if err != nil {
		glog.Errorf("Error: %v", err)
		return err
	}

	type params struct {
		Token    string
		Image    string
		CaBundle string
		TlsCrt   string
		TlsKey   string
	}

	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, params{
		Token:    token,
		Image:    machineControllerImage,
		CaBundle: certParms.caBundle,
		TlsCrt:   certParms.tlsCrt,
		TlsKey:   certParms.tlsKey,
	})
	if err != nil {
		return err
	}

	maxTries := 5
	for tries := 0; tries < maxTries; tries++ {
		err = deployConfig(tmplBuf.Bytes())
		if err == nil {
			return nil
		} else {
			if tries < maxTries-1 {
				glog.Info("Error scheduling machine controller. Will retry...\n")
				time.Sleep(3 * time.Second)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("couldn't start machine controller: %v\n", err)
	} else {
		return nil
	}
}

func deployConfig(manifest []byte) error {
	cmd := exec.Command("kubectl", "create", "-f", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(manifest)
	}()

	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	} else {
		return fmt.Errorf("couldn't create pod: %v, output: %s", err, string(out))
	}
}
