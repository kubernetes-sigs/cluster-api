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
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/cert/triple"
	"sigs.k8s.io/cluster-api/cloud/google/config"
)

var apiServerImage = "gcr.io/k8s-cluster-api/cluster-apiserver:0.0.3"
var controllerManagerImage = "gcr.io/k8s-cluster-api/controller-manager:0.0.3"
var machineControllerImage = "gcr.io/k8s-cluster-api/gce-machine-controller:0.0.9"

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

func getApiServerCerts() (*caCertParams, error) {
	const name = "clusterapi"
	const namespace = corev1.NamespaceDefault

	caKeyPair, err := triple.NewCA(fmt.Sprintf("%s-certificate-authority", name))
	if err != nil {
		return nil, fmt.Errorf("failed to create root-ca: %v", err)
	}

	apiServerKeyPair, err := triple.NewServerKeyPair(
		caKeyPair,
		fmt.Sprintf("%s.%s.svc", name, namespace),
		name,
		namespace,
		"cluster.local",
		[]string{},
		[]string{})
	if err != nil {
		return nil, fmt.Errorf("failed to create apiserver key pair: %v", err)
	}

	certParams := &caCertParams{
		caBundle: base64.StdEncoding.EncodeToString(cert.EncodeCertPEM(caKeyPair.Cert)),
		tlsKey:   base64.StdEncoding.EncodeToString(cert.EncodePrivateKeyPEM(apiServerKeyPair.Key)),
		tlsCrt:   base64.StdEncoding.EncodeToString(cert.EncodeCertPEM(apiServerKeyPair.Cert)),
	}

	return certParams, nil
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

func CreateIngressController(project string, clusterName string) error {
	tmpl, err := template.New("config").Parse(config.IngressControllerConfigTemplate)
	if err != nil {
		return err
	}

	type params struct {
		Project string
		NodeTag string
	}

	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, params{
		Project: project,
		NodeTag: clusterName + "-worker",
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
				glog.Infof("Error scheduling ingress controller. Will retry... %v\n", err)
				time.Sleep(3 * time.Second)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("couldn't start ingress controller: %v\n", err)
	} else {
		return nil
	}
}

func CreateDefaultStorageClass() error {
	tmpl, err := template.New("config").Parse(config.StorageClassConfigTemplate)
	if err != nil {
		return err
	}
	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, nil)
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
				glog.Info("Error creating default storage class. Will retry...\n")
				time.Sleep(3 * time.Second)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("couldn't create default storage class: %v\n", err)
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
