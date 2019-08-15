module sigs.k8s.io/cluster-api-provider-docker

go 1.12

require (
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/pkg/errors v0.8.1
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cluster-bootstrap v0.0.0-20190516232516-d7d78ab2cfe7 // indirect
	k8s.io/klog v0.4.0
	k8s.io/kubernetes v1.14.2
	sigs.k8s.io/cluster-api v0.0.0-20190725170330-835ee872f98d
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
	sigs.k8s.io/kind v0.4.0
	sigs.k8s.io/kustomize/v3 v3.0.3
)
