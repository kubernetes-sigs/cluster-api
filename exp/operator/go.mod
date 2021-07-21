module sigs.k8s.io/cluster-api/exp/operator

go 1.16

require (
	github.com/google/uuid v1.2.0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/klog/v2 v2.9.0
	sigs.k8s.io/cluster-api v0.0.0-00010101000000-000000000000
	sigs.k8s.io/controller-runtime v0.10.1
)

replace sigs.k8s.io/cluster-api => github.com/asalkeld/cluster-api v0.4.1-0.20210923065712-6ed39b7ef8f9
