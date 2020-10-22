module sigs.k8s.io/cluster-api/test/infrastructure/kubemark/hack/tools

go 1.15

require (
	github.com/Masterminds/semver v1.5.0
	github.com/golangci/golangci-lint v1.27.0
	github.com/google/go-containerregistry v0.1.4
	github.com/spf13/pflag v1.0.5
	sigs.k8s.io/cluster-api/hack/tools v0.0.0-20200130204219-ea93471ad47a
	sigs.k8s.io/controller-tools v0.4.1-0.20201002000720-57250aac17f6
)

replace sigs.k8s.io/cluster-api/hack/tools => ../../../../../hack/tools
