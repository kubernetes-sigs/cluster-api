module sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	k8s.io/api v0.0.0-20190703205437-39734b2a72fe
	k8s.io/apimachinery v0.0.0-20190704094733-8f6ac2502e51
	k8s.io/client-go v11.0.1-0.20190704100234-640d9f240853+incompatible
	k8s.io/cluster-bootstrap v0.0.0-20190703212826-5ad085674a4f
	k8s.io/klog v0.3.1
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a // indirect
	sigs.k8s.io/cluster-api v0.0.0-20190711133056-09e491e49d7c
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
)
