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

package terraform

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api/cloud/terraform/config"
)

var apiServerImage = "gcr.io/k8s-cluster-api/cluster-apiserver:0.0.3"
var controllerManagerImage = "gcr.io/k8s-cluster-api/controller-manager:0.0.3"
var machineControllerImage = "gcr.io/k8s-cluster-api/terraform-machine-controller:0.0.2"

func init() {
	if img, ok := os.LookupEnv("MACHINE_CONTROLLER_IMAGE"); ok {
		machineControllerImage = img
	}

	if img, ok := os.LookupEnv("CLUSTER_API_SERVER_IMAGE"); ok {
		apiServerImage = img
	}

	if img, ok := os.LookupEnv("CONTROLLER_MANAGER_IMAGE"); ok {
		controllerManagerImage = img
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
	const name = "clusterapi"
	const namespace = corev1.NamespaceDefault
	configDir, err := ioutil.TempDir("", "cert")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(configDir) // clean up

	if err := run("openssl", "req", "-x509",
		"-newkey", "rsa:2048",
		"-keyout", filepath.Join(configDir, "apiserver_ca.key"),
		"-out", filepath.Join(configDir, "apiserver_ca.crt"),
		"-days", "365",
		"-nodes",
		"-subj", fmt.Sprintf("/C=un/ST=st/L=l/O=o/OU=ou/CN=%s-certificate-authority", name)); err != nil {
		return nil, err
	}

	// Use <service-Name>.<Namespace>.svc as the domain Name for the certificate
	if err = run("openssl", "req",
		"-out", filepath.Join(configDir, "apiserver.csr"),
		"-new",
		"-newkey", "rsa:2048",
		"-nodes",
		"-keyout", filepath.Join(configDir, "apiserver.key"),
		"-subj", fmt.Sprintf("/C=un/ST=st/L=l/O=o/OU=ou/CN=%s.%s.svc", name, namespace)); err != nil {
		return nil, err
	}

	if err = run("openssl", "x509", "-req",
		"-days", "365",
		"-in", filepath.Join(configDir, "apiserver.csr"),
		"-CA", filepath.Join(configDir, "apiserver_ca.crt"),
		"-CAkey", filepath.Join(configDir, "apiserver_ca.key"),
		"-CAcreateserial",
		"-out", filepath.Join(configDir, "apiserver.crt")); err != nil {
		return nil, err
	}

	certParms := &caCertParams{
		caBundle: getBase64(filepath.Join(configDir, "apiserver_ca.crt")),
		tlsCrt:   getBase64(filepath.Join(configDir, "apiserver.crt")),
		tlsKey:   getBase64(filepath.Join(configDir, "apiserver.key")),
	}

	return certParms, nil
}

func CreateApiServerAndController(token string) error {
	tmpl, err := template.New("config").Parse(config.ClusterAPIDeployConfigTemplate)
	if err != nil {
		return err
	}

	certParms, err := getApiServerCerts()
	if err != nil {
		glog.Errorf("Error: %v", err)
		return err
	}

	type params struct {
		Token                  string
		APIServerImage         string
		ControllerManagerImage string
		MachineControllerImage string
		CABundle               string
		TLSCrt                 string
		TLSKey                 string
	}

	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, params{
		Token:                  token,
		APIServerImage:         apiServerImage,
		ControllerManagerImage: controllerManagerImage,
		MachineControllerImage: machineControllerImage,
		CABundle:               certParms.caBundle,
		TLSCrt:                 certParms.tlsCrt,
		TLSKey:                 certParms.tlsKey,
	})
	if err != nil {
		return err
	}

	ioutil.WriteFile("/tmp/pods.yaml", tmplBuf.Bytes(), 0644)

	maxTries := 30
	for tries := 0; tries < maxTries; tries++ {
		glog.Infof("Attempting to deploy cluster api config.")
		err = deployConfig(tmplBuf.Bytes())
		if err == nil {
			return nil
		} else {
			if tries < maxTries-1 {
				glog.Info("Error scheduling machine controller. Will retry...\n")
				time.Sleep(3 * time.Second)
			} else {
				glog.Info("Error scheduling machine controller. No more retries.")
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
