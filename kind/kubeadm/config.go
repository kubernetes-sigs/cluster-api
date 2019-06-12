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

package kubeadm

import (
	"bytes"
	"encoding/json"

	"k8s.io/kubernetes/pkg/kubelet/apis/config"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
)

const (
	KnownTokenID     = "abcdef"
	KnownTokenSecret = "0123456789abcdef"
	Token            = KnownTokenID + "." + KnownTokenSecret
)

func InitConifguration(version, name, controlPlaneEndpoint string) ([]byte, error) {
	configuration := &kubeadmv1beta1.InitConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InitConfiguration",
			APIVersion: "kubeadm.k8s.io/v1beta1",
		},
		BootstrapTokens: []kubeadmv1beta1.BootstrapToken{
			{
				Token: &kubeadmv1beta1.BootstrapTokenString{
					ID:     KnownTokenID,
					Secret: KnownTokenSecret,
				},
			},
		},
		LocalAPIEndpoint: kubeadmv1beta1.APIEndpoint{
			BindPort: int32(6443),
		},
		NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
			CRISocket: "/run/containerd/containerd.sock",
		},
	}
	clusterConfiguration := &kubeadmv1beta1.ClusterConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterConfiguration",
			APIVersion: "kubeadm.k8s.io/v1beta1",
		},
		APIServer: kubeadmv1beta1.APIServer{
			CertSANs: []string{"127.0.0.1"},
		},
		KubernetesVersion:    version,
		ClusterName:          name,
		ControlPlaneEndpoint: controlPlaneEndpoint,
		ControllerManager: kubeadmv1beta1.ControlPlaneComponent{
			ExtraArgs: map[string]string{
				"enable-hostpath-provisioner": "true",
			},
		},
		Networking: kubeadmv1beta1.Networking{
			PodSubnet: "10.244.0.0/16",
		},
	}
	kubeletConfiguration := &config.KubeletConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeletConfiguration",
			APIVersion: "kubelet.config.k8s.io/v1beta1",
		},
		ImageGCHighThresholdPercent: 100,
		EvictionHard: map[string]string{
			"nodefs.available":  "0%",
			"nodefs.inodesFree": "0%",
			"imagefs.available": "0%",
		},
	}
	initConfig, err := json.Marshal(configuration)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	clusterConfig, err := json.Marshal(clusterConfiguration)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kubeletConfig, err := json.Marshal(kubeletConfiguration)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// This hack is so good:
	return bytes.Join([][]byte{
		bytes.TrimSpace(initConfig),
		bytes.TrimSpace(clusterConfig),
		bytes.TrimSpace(kubeletConfig),
	}, []byte("\n---\n")), nil
}
