module sigs.k8s.io/cluster-api/test/infrastructure/docker/hack/tools

go 1.13

require (
	github.com/golangci/golangci-lint v1.27.0
	sigs.k8s.io/cluster-api/hack/tools v0.0.0-20200130204219-ea93471ad47a
	sigs.k8s.io/controller-tools v0.2.9
)

replace sigs.k8s.io/cluster-api/hack/tools => ../../../../../hack/tools
