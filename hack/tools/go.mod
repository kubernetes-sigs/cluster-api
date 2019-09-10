module sigs.k8s.io/cluster-api/hack/tools

go 1.12

require (
	github.com/golang/mock v1.3.1 // indirect
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/golangci/golangci-lint v1.17.1
	golang.org/x/sys v0.0.0-20190624142023-c5567b49c5d0 // indirect
	golang.org/x/tools v0.0.0-20190628153133-6cdbf07be9d0 // indirect
	k8s.io/code-generator v0.0.0-20190831074504-732c9ca86353
	sigs.k8s.io/controller-tools v0.2.0
	sigs.k8s.io/kubebuilder/docs/book/utils v0.0.0-20190903174343-de03361a00cb
	sigs.k8s.io/testing_frameworks v0.1.1
)

replace (
	// TODO(vincepri) Remove the following replace directives, needed for golangci-lint until
	// https://github.com/golangci/golangci-lint/issues/595 is fixed.
	github.com/go-critic/go-critic v0.0.0-20181204210945-1df300866540 => github.com/go-critic/go-critic v0.0.0-20190526074819-1df300866540
	github.com/golangci/errcheck v0.0.0-20181003203344-ef45e06d44b6 => github.com/golangci/errcheck v0.0.0-20181223084120-ef45e06d44b6
	github.com/golangci/go-tools v0.0.0-20180109140146-af6baa5dc196 => github.com/golangci/go-tools v0.0.0-20190318060251-af6baa5dc196
	github.com/golangci/gofmt v0.0.0-20181105071733-0b8337e80d98 => github.com/golangci/gofmt v0.0.0-20181222123516-0b8337e80d98
	github.com/golangci/gosec v0.0.0-20180901114220-66fb7fc33547 => github.com/golangci/gosec v0.0.0-20190211064107-66fb7fc33547
	github.com/golangci/ineffassign v0.0.0-20180808204949-42439a7714cc => github.com/golangci/ineffassign v0.0.0-20190609212857-42439a7714cc
	github.com/golangci/lint-1 v0.0.0-20180610141402-ee948d087217 => github.com/golangci/lint-1 v0.0.0-20190420132249-ee948d087217
	github.com/timakin/bodyclose => github.com/golangci/bodyclose v0.0.0-20190714144026-65da19158fa2
	mvdan.cc/unparam v0.0.0-20190124213536-fbb59629db34 => mvdan.cc/unparam v0.0.0-20190209190245-fbb59629db34
)
