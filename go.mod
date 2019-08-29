module sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cluster-bootstrap v0.0.0-20190516232516-d7d78ab2cfe7
	k8s.io/klog v0.4.0
	k8s.io/utils v0.0.0-20190809000727-6c36bc71fc4a // indirect
	sigs.k8s.io/cluster-api v0.0.0-20190829154355-58b8460b80c5
	sigs.k8s.io/controller-runtime v0.2.0
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190704095032-f4ca3d3bdf1d
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190704094733-8f6ac2502e51
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v0.0.0-20190829154355-58b8460b80c5
)
