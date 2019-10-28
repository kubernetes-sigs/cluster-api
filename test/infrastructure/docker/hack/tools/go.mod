module sigs.k8s.io/cluster-api/test/infrastructure/docker/hack/tools

go 1.12

require (
	sigs.k8s.io/cluster-api/hack/tools v0.0.0-20190830181856-67d897059593
	sigs.k8s.io/controller-tools v0.2.1
)

replace sigs.k8s.io/cluster-api/hack/tools => ../../../../../hack/tools
