/*
Copyright 2018 The Kubernetes Authors.

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

package deployer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"text/template"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/cert/triple"
)

var apiServerImage = "gcr.io/k8s-cluster-api/cluster-apiserver:0.0.5"

func init() {
	if img, ok := os.LookupEnv("CLUSTER_API_SERVER_IMAGE"); ok {
		apiServerImage = img
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

// GetApiServerYaml returns the clusterapi-apiserver manifest used for deployment
func GetApiServerYaml() (string, error) {
	tmpl, err := template.New("config").Parse(ClusterAPIAPIServerConfigTemplate)
	if err != nil {
		return "", err
	}

	certParms, err := getApiServerCerts()
	if err != nil {
		glog.Errorf("Error: %v", err)
		return "", err
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
		APIServerImage: apiServerImage,
		CABundle:       certParms.caBundle,
		TLSCrt:         certParms.tlsCrt,
		TLSKey:         certParms.tlsKey,
	})
	if err != nil {
		return "", err
	}

	return string(tmplBuf.Bytes()), nil
}
