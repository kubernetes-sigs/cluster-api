module sigs.k8s.io/cluster-api/hack/tools

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/drone/envsubst/v2 v2.0.0-20210730161058-179042472c46
	github.com/hashicorp/go-multierror v1.1.1
	github.com/joelanford/go-apidiff v0.1.0
	github.com/mikefarah/yq/v4 v4.13.5
	github.com/onsi/ginkgo v1.16.5
	github.com/pkg/errors v0.9.1
	github.com/sergi/go-diff v1.2.0 // indirect
	golang.org/x/exp v0.0.0-20211029160041-3396431c207b // indirect
	golang.org/x/tools v0.1.8-0.20211029000441-d6a9af8af023
	gotest.tools/gotestsum v1.6.4
	k8s.io/code-generator v0.22.2
	k8s.io/klog/v2 v2.9.0
	sigs.k8s.io/controller-runtime/tools/setup-envtest v0.0.0-20211025141024-c73b143dc503
	sigs.k8s.io/controller-tools v0.7.0
	sigs.k8s.io/kubebuilder/docs/book/utils v0.0.0-20211028165026-57688c578b5d
)
