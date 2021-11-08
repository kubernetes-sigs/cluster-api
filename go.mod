module sigs.k8s.io/cluster-api

go 1.16

require (
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/coredns/corefile-migration v1.0.13
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/drone/envsubst/v2 v2.0.0-20210615175204-7bf45dbf5372
	github.com/evanphx/json-patch/v5 v5.6.0
	github.com/fatih/color v1.13.0
	github.com/go-logr/logr v0.4.0
	github.com/gobuffalo/flect v0.2.3
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v33 v33.0.0
	github.com/google/gofuzz v1.2.0
	github.com/gosuri/uitable v0.0.4
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.9.0
	go.etcd.io/etcd/api/v3 v3.5.0
	go.etcd.io/etcd/client/v3 v3.5.0
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f
	google.golang.org/grpc v1.40.0
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/apiserver v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/cluster-bootstrap v0.22.2
	k8s.io/component-base v0.22.2
	k8s.io/klog/v2 v2.9.0
	k8s.io/kubectl v0.22.2
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/controller-runtime v0.10.3-0.20211011182302-43ea648ec318
	sigs.k8s.io/yaml v1.3.0
)

replace github.com/evanphx/json-patch/v5 => github.com/sbueringer/json-patch v5.6.1-0.20211108121231-34851406de32+incompatible
