module sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cluster-bootstrap v0.0.0-20190516232516-d7d78ab2cfe7
	k8s.io/klog v0.3.3
	sigs.k8s.io/cluster-api v0.0.0-20190725170330-835ee872f98d
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
)
