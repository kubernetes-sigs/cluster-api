module sigs.k8s.io/cluster-api

go 1.12

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/pkg/errors v0.8.1
	github.com/sergi/go-diff v1.0.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	golang.org/x/net v0.0.0-20190909003024-a7b16738d86b
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/apiserver v0.0.0-20190918200908-1e17798da8c1
	k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/cluster-bootstrap v0.0.0-20190516232516-d7d78ab2cfe7
	k8s.io/component-base v0.0.0-20190918200425-ed2f0867c778
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20190809000727-6c36bc71fc4a
	sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm v0.1.5 // indirect
	sigs.k8s.io/controller-runtime v0.3.0
)
