module sigs.k8s.io/cluster-api/test/infrastructure/docker

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/onsi/gomega v1.11.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	k8s.io/component-base v0.21.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	sigs.k8s.io/cluster-api v0.3.3
	sigs.k8s.io/controller-runtime v0.9.0-beta.5
	sigs.k8s.io/kind v0.9.0
	sigs.k8s.io/yaml v1.2.0
)

replace sigs.k8s.io/cluster-api => ../../..
