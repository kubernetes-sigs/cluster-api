module sigs.k8s.io/cluster-api/test/infrastructure/docker/hack/tools

go 1.16

require (
	github.com/golangci/golangci-lint v1.38.0
	k8s.io/code-generator v0.21.0-beta.0
	sigs.k8s.io/cluster-api/hack/tools v0.0.0-20200130204219-ea93471ad47a
	sigs.k8s.io/controller-tools v0.5.0
)

replace sigs.k8s.io/cluster-api/hack/tools => ../../../../../hack/tools
