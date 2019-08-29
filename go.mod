module sigs.k8s.io/cluster-api-provider-docker

go 1.12

require (
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/pkg/errors v0.8.1
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8 // indirect
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/sys v0.0.0-20190621203818-d432491b9138 // indirect
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v0.4.0
	k8s.io/kube-openapi v0.0.0-20190603182131-db7b694dc208 // indirect
	sigs.k8s.io/cluster-api v0.0.0-20190829144357-1063658f9b58
	sigs.k8s.io/controller-runtime v0.2.0
	sigs.k8s.io/kind v0.4.0
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190704095032-f4ca3d3bdf1d
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190704094733-8f6ac2502e51
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v0.0.0-20190829144357-1063658f9b58
)
