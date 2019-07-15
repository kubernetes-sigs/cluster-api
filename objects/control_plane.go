/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package objects

import (
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

const namespace = "docker-provider-system"

// GetNamespace returns a  "docker-provider-system" namespace object
func GetNamespace() core.Namespace {
	return core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Labels: map[string]string{"controller-tools.k8s.io": "1.0"},
			Name:   namespace,
		},
	}
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

// GetStatefulSet returns a statefulset for running CAPD with the given image name
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
								"-v=3",
								"-logtostderr=true",
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
							Effect: core.TaintEffectNoSchedule,
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
