module sigs.k8s.io/cluster-api

go 1.13

require (
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/coredns/corefile-migration v1.0.7
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/drone/envsubst v1.0.3-0.20200709223903-efdb65b94e5a
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/gogo/protobuf v1.3.1
	github.com/google/go-cmp v0.4.1
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/gofuzz v1.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/client_model v0.2.0
	github.com/spf13/cobra v0.0.6
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/grpc v1.26.0
	k8s.io/api v0.18.5
	k8s.io/apiextensions-apiserver v0.18.5
	k8s.io/apimachinery v0.18.5
	k8s.io/apiserver v0.18.5
	k8s.io/client-go v0.18.5
	k8s.io/cluster-bootstrap v0.18.5
	k8s.io/component-base v0.18.5
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/controller-runtime v0.6.1
	sigs.k8s.io/kind v0.7.1-0.20200303021537-981bd80d3802
	sigs.k8s.io/yaml v1.2.0
)

// TODO(vincepri): Remove this replace once upstream requires this commit directly.
// See context in https://github.com/kubernetes-sigs/controller-runtime/pull/985.
replace github.com/evanphx/json-patch => github.com/evanphx/json-patch v0.0.0-20190815234213-e83c0a1c26c8
