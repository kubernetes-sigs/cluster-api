module sigs.k8s.io/cluster-api/test/infrastructure/docker

go 1.13

require (
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.9.0
	gopkg.in/yaml.v3 v3.0.0-20191120175047-4206685974f2
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/klog v1.0.0
	sigs.k8s.io/cluster-api v0.3.0-rc.2.0.20200302175844-3011d8c2580c
	sigs.k8s.io/cluster-api/test/framework v0.0.0-20200212174651-13d44c484542
	sigs.k8s.io/controller-runtime v0.5.0
	sigs.k8s.io/kind v0.7.0
)

replace (
	sigs.k8s.io/cluster-api => ../../..
	sigs.k8s.io/cluster-api/test/framework => ../../framework
)
