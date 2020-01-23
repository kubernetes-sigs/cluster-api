package generators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

const (
	outWithLeaderElectionEnabled = `apiVersion: v1
kind: Namespace
metadata:
  labels:
    control-plane: controller-manager
  name: capi-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    control-plane: controller-manager
  name: capi-controller-manager
  namespace: capi-system
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:8080/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.4.1
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
      - args:
        - --metrics-addr=127.0.0.1:8080
        - --enable-leader-election
        command:
        - /manager
        image: controller:latest
        name: manager
      terminationGracePeriodSeconds: 10
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
`
	outWithLeaderElectionDisabled = `apiVersion: v1
kind: Namespace
metadata:
  labels:
    control-plane: controller-manager
  name: capi-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    control-plane: controller-manager
  name: capi-controller-manager
  namespace: capi-system
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:8080/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.4.1
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
      - args:
        - --metrics-addr=127.0.0.1:8080
        - --enable-leader-election=false
        command:
        - /manager
        image: controller:latest
        name: manager
      terminationGracePeriodSeconds: 10
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
`
)

func TestClusterAPI_Manifests(t *testing.T) {
	tests := []struct {
		name string
		capi *ClusterAPI
		want string
	}{
		{
			name: "leader election enabled",
			capi: &ClusterAPI{
				KustomizePath:         "testdata/capi/",
				Version:               "",
				DisableLeaderElection: false,
			},
			want: outWithLeaderElectionEnabled,
		},
		{
			name: "leader election disabled",
			capi: &ClusterAPI{
				KustomizePath:         "testdata/capi/",
				Version:               "",
				DisableLeaderElection: true,
			},
			want: outWithLeaderElectionDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := tt.capi.Manifests(context.TODO())
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(got)).To(Equal(tt.want))
		})
	}
}
