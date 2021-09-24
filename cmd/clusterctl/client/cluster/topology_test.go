/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"testing"

	. "github.com/onsi/gomega"
)

var f = []byte(`
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-cluster1
  namespace: default
  labels:
    cni: kindnet
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  topology:
    class: clusterclass1
    version: v1.21.2
    controlPlane:
      replicas: 1
    workers:
      machineDeployments:
        - name: md1
          class: "md-class-1"
          replicas: 1
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: clusterclass1
spec:
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerClusterTemplate
      name: clusterclass1-infrastructure-cluster-template
  controlPlane:
    metadata:
      labels:
        class: "clusterclass1-controlplane-label"
      annotations:
        class: "clusterclass1-controlplane-annotation"
    ref:
      apiVersion: controlplane.cluster.x-k8s.io/v1beta1
      kind: KubeadmControlPlaneTemplate
      name: clusterclass1-controlplane-template
    machineInfrastructure:
      ref:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: DockerMachineTemplate
        name: clusterclass1-controlplane-machinetemplate
  workers:
    machineDeployments:
    - class: "md-class-1"
      template:
        metadata:
          labels:
            class: "clusterclass1-md-class-1-label"
          annotations:
            class: "clusterclass1-md-class-1-annotation"
        bootstrap:
          ref:
            apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
            kind: KubeadmConfigTemplate
            name: clusterclass1-md-class-1-bootstraptemplate
        infrastructure:
          ref:
            apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
            kind: DockerMachineTemplate
            name: clusterclass1-md-class-1-machinetemplate
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerClusterTemplate
metadata:
  name: clusterclass1-infrastructure-cluster-template
spec:
  template:
    spec: {}
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlaneTemplate
metadata:
  name: clusterclass1-controlplane-template
spec:
  template:
    spec:
      version: v1.22.0  # <<< Web hooks or type improvent; this is an instance specific field
      machineTemplate:  # <<< Web hooks or type improvent; this field is driven by clusterclass.spec.controlplane.machineinfrastructure.ref
        infrastructureRef:
          apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
          kind: DockerMachineTemplate
          name: clusterclass1-controlplane-machinetemplate
      kubeadmConfigSpec:
        clusterConfiguration:
          apiServer:
            certSANs:
            - localhost
            - 127.0.0.1
            - 0.0.0.0
          controllerManager:
            extraArgs:
              enable-hostpath-provisioner: "true"
        initConfiguration:
          nodeRegistration:
            kubeletExtraArgs:
              # We have to pin the cgroupDriver to cgroupfs as kubeadm >=1.21 defaults to systemd
              # kind will implement systemd support in: https://github.com/kubernetes-sigs/kind/issues/1726
              cgroup-driver: cgroupfs
              eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
        joinConfiguration:
          nodeRegistration:
            kubeletExtraArgs:
              # We have to pin the cgroupDriver to cgroupfs as kubeadm >=1.21 defaults to systemd
              # kind will implement systemd support in: https://github.com/kubernetes-sigs/kind/issues/1726
              cgroup-driver: cgroupfs
              eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachineTemplate
metadata:
  name: clusterclass1-controlplane-machinetemplate
spec:
  template:
    spec: {}
---
apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
kind: KubeadmConfigTemplate
metadata:
  name: clusterclass1-md-class-1-bootstraptemplate
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
            # We have to pin the cgroupDriver to cgroupfs as kubeadm >=1.21 defaults to systemd
            # kind will implement systemd support in: https://github.com/kubernetes-sigs/kind/issues/1726
            cgroup-driver: cgroupfs
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachineTemplate
metadata:
  name: clusterclass1-md-class-1-machinetemplate
spec:
  template:
    spec: {}
`)

func Test_topologyClient_DryRun(t *testing.T) {
	type args struct {
		in *DryRunInput
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "test1",
			args: args{
				in: &DryRunInput{File: f},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			tc := &topologyClient{}
			_, err := tc.DryRun(tt.args.in)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}
