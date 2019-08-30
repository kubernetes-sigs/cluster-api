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

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestKeyPairToSecret(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "myns",
			Name:      "mycluster",
		},
	}
	config := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "myns",
			Name:      "myconfig",
			UID:       types.UID("myuid"),
		},
	}
	name := "foo"
	keyPair := &KeyPair{
		Cert: []byte("cert"),
		Key:  []byte("key"),
	}

	s := KeyPairToSecret(cluster, config, name, keyPair)
	assert.Equal(t, "myns", s.Namespace)
	assert.Equal(t, "mycluster-foo", s.Name)
	assert.Equal(t, "mycluster", s.Labels[clusterv1.MachineClusterLabelName])
	if assert.Len(t, s.OwnerReferences, 1) {
		assert.Equal(t, metav1.OwnerReference{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "KubeadmConfig",
			Name:       "myconfig",
			UID:        "myuid",
		}, s.OwnerReferences[0])
	}
	if assert.Len(t, s.Data, 2) {
		assert.Equal(t, []byte("cert"), s.Data[secret.TLSCrtDataName])
		assert.Equal(t, []byte("key"), s.Data[secret.TLSKeyDataName])
	}
}
