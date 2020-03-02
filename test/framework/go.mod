module sigs.k8s.io/cluster-api/test/framework

go 1.13

require (
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.9.0
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/cluster-api v0.3.0-rc.2.0.20200302175844-3011d8c2580c
	sigs.k8s.io/controller-runtime v0.5.0
	sigs.k8s.io/kind v0.7.0
	sigs.k8s.io/yaml v1.1.0
)
