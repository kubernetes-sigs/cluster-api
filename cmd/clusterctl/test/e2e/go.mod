module sigs.k8s.io/cluster-api/cmd/clusterctl/test/e2e

go 1.13

require (
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.9.0
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/cluster-api v0.3.0-rc.2.0.20200302175844-3011d8c2580c
	sigs.k8s.io/cluster-api/test/framework v0.0.0-20200125173702-54f26d7fd2b5
	sigs.k8s.io/cluster-api/test/infrastructure/docker v0.0.0-20200125173702-54f26d7fd2b5
	sigs.k8s.io/controller-runtime v0.5.0
	sigs.k8s.io/kind v0.7.0
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	sigs.k8s.io/cluster-api => ../../../..
	sigs.k8s.io/cluster-api/test/framework => ../../../../test/framework
)
