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

package cert

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"k8s.io/klog"
)

type CertificateAuthority struct {
	Certificate []byte
	PrivateKey  []byte
}

// Load takes a given path to what is presumed to be a valid certificate authority and attempts to load both the
// certificate and private key. If the path is a directory then it is assumed that there are two files "ca.crt" and
// "ca.key" which represent the certificate and private key respectively.
//
// If the argument is not a directory, it must be a file ending in either of the .crt or .key extensions. Load will
// read the given file and attempt to load the other associated file. For example, if the path given is /path/to/my-ca.crt,
// then load will attempt to load the private key file at /path/to/my-ca.key.
func Load(caPath string) (*CertificateAuthority, error) {
	klog.Infof("Loading certificate authority from %v", caPath)
	certPath, keyPath, err := certPathToCertAndKeyPaths(caPath)
	if err != nil {
		return nil, err
	}
	certMaterial, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read cert %v", certPath)
	}
	keyMaterial, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read key %v", keyPath)
	}
	ca := CertificateAuthority{
		Certificate: certMaterial,
		PrivateKey:  keyMaterial,
	}
	return &ca, nil
}

func certPathToCertAndKeyPaths(caPath string) (string, string, error) {
	fi, err := os.Stat(caPath)
	if err != nil {
		return "", "", err
	}
	var certPath, keyPath string
	if fi.IsDir() {
		certPath = filepath.Join(caPath, "ca.crt")
		keyPath = filepath.Join(caPath, "ca.key")
	} else {
		ext := filepath.Ext(caPath)
		switch ext {
		case ".crt":
			certPath = caPath
			keyPath = caPath[0:len(caPath)-len(ext)] + ".key"
		case ".key":
			certPath = caPath[0:len(caPath)-len(ext)] + ".crt"
			keyPath = caPath
		default:
			return "", "", errors.Errorf("unable to use certificate authority, not directory, .crt, or .key file: %v", caPath)
		}
	}
	if _, err := os.Stat(certPath); err != nil {
		return "", "", errors.Wrapf(err, "unable to use certificate file: %v", certPath)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return "", "", errors.Wrapf(err, "unable to use key file: %v", keyPath)
	}
	return certPath, keyPath, err
}
