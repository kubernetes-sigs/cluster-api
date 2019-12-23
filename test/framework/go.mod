module sigs.k8s.io/cluster-api/test/framework

go 1.13

require (
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/pkg/errors v0.8.1
	k8s.io/api v0.0.0-20191121015604-11707872ac1c
	k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604 // indirect
	k8s.io/apimachinery v0.0.0-20191121015412-41065c7a8c2a
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/cluster-api v0.2.6-0.20191223162332-fd807a3d843b
	sigs.k8s.io/controller-runtime v0.4.0
)

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
