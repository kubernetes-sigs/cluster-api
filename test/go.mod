module sigs.k8s.io/cluster-api/test

go 1.16

replace sigs.k8s.io/cluster-api => ../

require (
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/containerd v1.5.2 // indirect
	github.com/docker/docker v20.10.7+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/go-logr/logr v0.4.0
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/component-base v0.22.2
	k8s.io/klog/v2 v2.9.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/cluster-api v0.0.0-00010101000000-000000000000
	sigs.k8s.io/controller-runtime v0.10.3-0.20211011182302-43ea648ec318
	sigs.k8s.io/kind v0.11.1
	sigs.k8s.io/yaml v1.3.0
)

replace sigs.k8s.io/controller-runtime => github.com/fabriziopandini/controller-runtime v0.2.0-beta.1.0.20211105090959-708d5397c4f3
