package objects

import (
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

const namespace = "docker-provider-system"

var Namespace = core.Namespace{
	ObjectMeta: meta.ObjectMeta{
		Labels: map[string]string{"controller-tools.k8s.io": "1.0"},
		Name:   namespace,
	},
}

var (
	controlPlaneLabel = map[string]string{"control-plane": "controller-manager"}
	hostPathSocket    = core.HostPathSocket
	hostPathDirectory = core.HostPathDirectory
)

const (
	dockerSockVolumeName = "dockersock"
	dockerSockPath       = "/var/run/docker.sock"
	dockerLibVolumeName  = "dockerlib"
	dockerLibPath        = "/var/lib/docker"
)

func GetStatefulSet(image string) apps.StatefulSet {
	return apps.StatefulSet{
		ObjectMeta: meta.ObjectMeta{
			Labels:    controlPlaneLabel,
			Name:      "docker-provider-controller-manager",
			Namespace: namespace,
		},
		Spec: apps.StatefulSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: controlPlaneLabel,
			},
			ServiceName: "docker-provider-controller-manager-service",
			Template: core.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Labels: controlPlaneLabel,
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:  "capd-manager",
							Image: image,
							Command: []string{
								"capd-manager",
							},
							VolumeMounts: []core.VolumeMount{
								{
									MountPath: dockerSockPath,
									Name:      dockerSockVolumeName,
								},
								{
									MountPath: dockerLibPath,
									Name:      dockerLibVolumeName,
								},
							},
						},
					},
					Volumes: []core.Volume{
						{
							Name: dockerSockVolumeName,
							VolumeSource: core.VolumeSource{
								HostPath: &core.HostPathVolumeSource{
									Path: dockerSockPath,
									Type: &hostPathSocket,
								},
							},
						},
						{
							Name: dockerLibVolumeName,
							VolumeSource: core.VolumeSource{
								HostPath: &core.HostPathVolumeSource{
									Path: dockerLibPath,
									Type: &hostPathDirectory,
								},
							},
						},
					},
					Tolerations: []core.Toleration{
						{
							Key:    constants.LabelNodeRoleMaster,
							Effect: core.TaintEffectNoExecute,
						},
						{
							Key:      "CriticalAddonsOnly",
							Operator: core.TolerationOpExists,
						},
						{
							Key:      "node.alpha.kubernetes.io/notReady",
							Operator: core.TolerationOpExists,
							Effect:   core.TaintEffectNoExecute,
						},
						{
							Key:      "node.alpha.kubernetes.io/unreachable",
							Operator: core.TolerationOpExists,
							Effect:   core.TaintEffectNoExecute,
						},
					},
				},
			},
		},
	}
}
