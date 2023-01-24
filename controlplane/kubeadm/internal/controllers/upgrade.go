/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"context"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/patch"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
        "k8s.io/apimachinery/pkg/runtime/schema"
)

func (r *KubeadmControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KubeadmControlPlane,
	controlPlane *internal.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	/* if kcp.Spec.RolloutStrategy == nil || kcp.Spec.RolloutStrategy.RollingUpdate == nil {
		return ctrl.Result{}, errors.New("rolloutStrategy is not set")
	} */

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))
		return ctrl.Result{}, err
	}

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
	}

	if err := workloadCluster.ReconcileKubeletRBACRole(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC role")
	}

	if err := workloadCluster.ReconcileKubeletRBACBinding(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC binding")
	}

	// Ensure kubeadm cluster role  & bindings for v1.18+
	// as per https://github.com/kubernetes/kubernetes/commit/b117a928a6c3f650931bdac02a41fca6680548c4
	if err := workloadCluster.AllowBootstrapTokensToGetNodes(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set role and role binding for kubeadm")
	}

	if err := workloadCluster.UpdateKubernetesVersionInKubeadmConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update the kubernetes version in the kubeadm config map")
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		// We intentionally only parse major/minor/patch so that the subsequent code
		// also already applies to beta versions of new releases.
		parsedVersionTolerant, err := version.ParseMajorMinorPatchTolerant(kcp.Spec.Version)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
		}
		// Get the imageRepository or the correct value if nothing is set and a migration is necessary.
		imageRepository := internal.ImageRepositoryFromClusterConfig(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration, parsedVersionTolerant)

		if err := workloadCluster.UpdateImageRepositoryInKubeadmConfigMap(ctx, imageRepository, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the image repository in the kubeadm config map")
		}
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil && kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		if err := workloadCluster.UpdateEtcdVersionInKubeadmConfigMap(ctx, meta.ImageRepository, meta.ImageTag, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd version in the kubeadm config map")
		}

		extraArgs := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ExtraArgs
		if err := workloadCluster.UpdateEtcdExtraArgsInKubeadmConfigMap(ctx, extraArgs, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd extra args in the kubeadm config map")
		}
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		if err := workloadCluster.UpdateAPIServerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update api server in the kubeadm config map")
		}

		if err := workloadCluster.UpdateControllerManagerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update controller manager in the kubeadm config map")
		}

		if err := workloadCluster.UpdateSchedulerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update scheduler in the kubeadm config map")
		}
	}

	if err := workloadCluster.UpdateKubeletConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to upgrade kubelet config map")
	}

	switch kcp.Spec.RolloutStrategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		// We can ignore MaxUnavailable because we are enforcing health checks before we get here.
		maxNodes := *kcp.Spec.Replicas + int32(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntValue())
		if int32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
		}
		return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, machinesRequireUpgrade)
        case controlplanev1.InPlaceUpdateStrategyType:
                logger.Info("RolloutStrategy type is InPlaceUpdateStrategyType")
                var err error
                for _, value := range machinesRequireUpgrade {
                        _, err = r.inPlaceUpgradeMachine(ctx, cluster, kcp, controlPlane, value);
                        break;
                }
                return ctrl.Result{}, err
	default:
		logger.Info("RolloutStrategy type is not set to RollingUpdateStrategyType, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}

func (r *KubeadmControlPlaneReconciler) inPlaceUpgradeMachine(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KubeadmControlPlane,
	controlPlane *internal.ControlPlane,
	machineToUpgrade *clusterv1.Machine,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	if machineToUpgrade.Status.NodeRef == nil {
		return ctrl.Result{}, errors.New("machineToUpgrade does not have NodeRef")
	} 

	// check if upgrader pod exists, and check for its status
	// Using a typed object.
	pod := &corev1.Pod{}
	// c is a created client.
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: "eksa-system",
		Name:      "upgrader-pod",
	}, pod)

	if err == nil {
		logger.Info("**************** Found pod", pod.Name, " **********************")
		for _, contStatus := range pod.Status.ContainerStatuses {
			if (contStatus.State.Terminated == nil || contStatus.State.Terminated.Reason != "Completed") {
				return ctrl.Result{}, errors.New("upgrader pod is not ready yet")
			}
		}
	} else {
		configMap := &corev1.ConfigMap{
  			ObjectMeta: metav1.ObjectMeta{
    				Name:      "coredns",
    				Namespace: "kube-system",
  			},
		}
		logger.Info("Deleting coredns configmap -- pooja")
		err = r.Client.Delete(ctx, configMap)

		if err != nil {
			// return ctrl.Result{}, errors.Wrap(err, "failed to delete coredns configMap")
			logger.Info("failed to delete coredns configMap")
		} 
		logger.Info("Done deleting coredns configMap")
		/* configMap := &corev1.ConfigMap {
    			Metadata: &metav1.ObjectMeta {
        			Name:      k8s.String("coredns"),
        			Namespace: k8s.String("kube-system"),
    			},
		}
		if err := r.Client.Delete(ctx, configMap); err != nil {
		} 
		if err := client.Delete().Namespace("kube-system").Resource("configmaps").Name("coredns").Do(ctx).Error(); err != nil {
			logger.Error("Unable to delete coredns config map. CoreDNS will not be upgraded: ", err)
		
		} */
		//hostnameLabelValue := r.getHostnameLabelValue(ctx, cluster, machineToUpgrade)
		u := &unstructured.Unstructured{}
		u.Object = r.getPodDefinition(ctx, machineToUpgrade)
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Pod",
			Version: "v1",
		})
			
		logger.Info("Creating upgrader pod -- pooja")
		// c is a created client.
		err = r.Client.Create(ctx, u)

		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to create upgrader pod")
		} 
		logger.Info("Done Creating upgrader pod")
		return ctrl.Result{}, errors.New("waiting for upgrader pod to be ready")
	}

	// Wait for the upgrader pod to finish

	logger.Info("Versions are", "version", machineToUpgrade.Status.NodeInfo.KubeletVersion)
	patchHelper, err := patch.NewHelper(machineToUpgrade, r.Client)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	// Now patch the Machine object with the new k8s version from NodeInfo
	machineToUpgrade.Spec.Version = &(machineToUpgrade.Status.NodeInfo.KubeletVersion)

	if err := patchHelper.Patch(ctx, machineToUpgrade); err != nil {
		conditions.MarkFalse(machineToUpgrade, clusterv1.MachineNodeHealthyCondition, clusterv1.DeletionFailedReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, errors.Wrap(err, "failed to patch Machine")
	}

	logger.Info("Machine Patching Done", "Version", *(machineToUpgrade.Spec.Version))
	return ctrl.Result{}, nil
}

func (r *KubeadmControlPlaneReconciler) getPodDefinition(ctx context.Context, machineToUpgrade *clusterv1.Machine) map[string]interface{}{
	logger := ctrl.LoggerFrom(ctx)
	//upgradeCommands := "apt-mark unhold kubelet && apt-get update && apt-get install -y kubelet=1.23.14-00 && apt-mark hold kubelet; apt-mark unhold kubeadm && apt-get update && apt-get install -y kubeadm=1.23.14-00 && apt-mark hold kubeadm; kubeadm version; kubeadm upgrade apply v1.23.14; systemctl daemon-reload; systemctl restart kubelet; systemctl status kubelet"

	newVersionString := "v1.23.15-eks-1-23-11"
	kubeadmUrl := "https://distro.eks.amazonaws.com/kubernetes-1-23/releases/11/artifacts/kubernetes/v1.23.15/bin/linux/amd64/kubeadm"
	kubeletUrl := "https://distro.eks.amazonaws.com/kubernetes-1-23/releases/11/artifacts/kubernetes/v1.23.15/bin/linux/amd64/kubelet"
	corednsURI := "public.ecr.aws\\/eks-distro\\/coredns"
	etcdURI := "public.ecr.aws\\/eks-distro\\/etcd-io"
	newCoreDNSImageTag := "v1.8.7-eks-1-23-11"
	newEtcdImageTag := "v3.5.6-eks-1-23-11"


	upgradeCommands := "/usr/bin/cp /usr/bin/kubeadm /usr/bin/kubeadm.bk && /usr/bin/cp /usr/bin/kubelet /usr/bin/kubelet.bk"
	upgradeCommands += " && /usr/bin/curl " + kubeadmUrl + " -o /usr/bin/kubeadm"
	upgradeCommands += " && /usr/bin/cp /var/run/kubeadm/kubeadm.yaml /var/run/kubeadm/kubeadm.yaml.bk"
	upgradeCommands += " && sed -i -e 's:^kubernetesVersion\\:.*$:kubernetesVersion\\: " + newVersionString + ":g ; /imageRepository: " + corednsURI + "/{n;s/imageTag: .*/imageTag: " + newCoreDNSImageTag + "/} ; /imageRepository: " + etcdURI + "/{n;s/imageTag: .*/imageTag: " + newEtcdImageTag + "/}' /var/run/kubeadm/kubeadm.yaml"
	upgradeCommands += " && kubeadm init phase addon all --config /var/run/kubeadm/kubeadm.yaml && kubeadm init phase etcd local --config /var/run/kubeadm/kubeadm.yaml"
	upgradeCommands += " && kubeadm upgrade apply " + newVersionString + " --ignore-preflight-errors=CoreDNSUnsupportedPlugins,CoreDNSMigration --force"
	upgradeCommands += "; /usr/bin/curl " + kubeletUrl + " -o /usr/bin/kubelet.new && systemctl stop kubelet && /usr/bin/cp /usr/bin/kubelet.new /usr/bin/kubelet && systemctl daemon-reload; systemctl restart kubelet; systemctl status kubelet"
		logger.Info("********** Upgrading **********", upgradeCommands)
	ret := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "upgrader-pod",
			"namespace": "eksa-system",
		},
		"spec": map[string]interface{}{
			"nodeName": machineToUpgrade.Status.NodeRef.Name,
			"containers": []map[string]interface{}{
				{
					"name":  "upgrader",
					"image": "public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-nsenter:latest.2",
					"command": []string {
							"nsenter",
						},
					"args": []string {
							"--target",
							"1",
							"--mount",
							"--uts",
							"--ipc",
							"--net",
							"/bin/sh",
							"-c",
							upgradeCommands,
							//"apt-mark unhold kubelet && apt-get update && apt-get install -y kubelet=1.23.14-00 && apt-mark hold kubelet; apt-mark unhold kubeadm && apt-get update && apt-get install -y kubeadm=1.23.14-00 && apt-mark hold kubeadm; kubeadm version; kubeadm upgrade apply v1.23.14 --ignore-preflight-errors=CoreDNSUnsupportedPlugins,CoreDNSMigration; systemctl daemon-reload; systemctl restart kubelet; systemctl status kubelet",
						},
					"securityContext": map[string]interface{}{
						"privileged": true,
					},
					"volumeMounts": []map[string]interface{}{
						{
							"name": "root",
							"mountPath": "/newRoot",
						},
					},
				},
			},
			"restartPolicy": "Never",
			"hostPID": true,
			"volumes": []map[string]interface{}{
				{
					"name": "root",
					"hostPath": map[string]interface{}{
						"path": "/",
					},
				},
			},
			/* "nodeSelector": map[string]interface{}{
				"hostname": hostnameLabelValue,
			},*/
		},
	}
	return ret
}
