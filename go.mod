module sigs.k8s.io/cluster-api

go 1.12

require (
	github.com/Azure/go-autorest/autorest v0.9.2 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.8.0 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/golang/groupcache v0.0.0-20181024230925-c65c006176ff // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	github.com/sergi/go-diff v1.0.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	golang.org/x/net v0.0.0-20190812203447-cdfb69ac37fc
	k8s.io/api v0.0.0-20191004120104-195af9ec3521
	k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8
	k8s.io/apiserver v0.0.0-20190918200908-1e17798da8c1
	k8s.io/client-go v0.0.0-20191004120905-f06fe3961ca9
	k8s.io/code-generator v0.0.0-20191004115455-8e001e5d1894
	k8s.io/component-base v0.0.0-20191004121439-41066ddd0b23
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20191010214722-8d271d903fe4
	sigs.k8s.io/controller-runtime v0.3.0
	sigs.k8s.io/controller-tools v0.2.2
	sigs.k8s.io/testing_frameworks v0.1.1
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
)
