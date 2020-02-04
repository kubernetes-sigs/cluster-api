module sigs.k8s.io/cluster-api/test/infrastructure/docker/hack/tools

go 1.13

require (
	github.com/golangci/golangci-lint v1.23.3
	sigs.k8s.io/cluster-api/hack/tools v0.0.0-20190830181856-67d897059593
	sigs.k8s.io/controller-tools v0.2.4
)

replace sigs.k8s.io/cluster-api/hack/tools => ../../../../../hack/tools
