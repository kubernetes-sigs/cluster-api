module sigs.k8s.io/cluster-api/test/framework

go 1.13

require (
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/pkg/errors v0.8.1
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	sigs.k8s.io/cluster-api v0.0.0-00010101000000-000000000000
	sigs.k8s.io/controller-runtime v0.3.0
)

replace sigs.k8s.io/cluster-api => ../..
