/*
Copyright 2019 The Kubernetes Authors.

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

package certs

import "errors"

// Validate checks that all KeyPairs are valid
func (c *Certificates) Validate() error {
	if !c.ClusterCA.isValid() {
		return errors.New("CA cert material is missing cert/key")
	}

	if !c.EtcdCA.isValid() {
		return errors.New("ETCD CA cert material is missing cert/key")
	}

	if !c.FrontProxyCA.isValid() {
		return errors.New("FrontProxy CA cert material is missing cert/key")
	}

	if !c.ServiceAccount.isValid() {
		return errors.New("ServiceAccount cert material is missing cert/key")
	}

	return nil
}

func (kp *KeyPair) isValid() bool {
	return kp.Cert != nil && kp.Key != nil
}
