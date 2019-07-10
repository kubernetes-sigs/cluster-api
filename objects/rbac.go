package objects

import (
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const clusterRoleName = "docker-provider-manager-role"

func GetClusterRole() rbac.ClusterRole {
	return rbac.ClusterRole{
		ObjectMeta: meta.ObjectMeta{
			Name: clusterRoleName,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{
					capi.SchemeGroupVersion.Group,
				},
				Resources: []string{
					"clusters",
					"clusters/status",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
			},
			{
				APIGroups: []string{
					capi.SchemeGroupVersion.Group,
				},
				Resources: []string{
					"machines",
					"machines/status",
					"machinedeployments",
					"machinedeployments/status",
					"machinesets",
					"machinesets/status",
					"machineclasses",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
			},
			{
				APIGroups: []string{
					core.GroupName,
				},
				Resources: []string{
					"nodes",
					"events",
					"secrets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
			},
		},
	}
}

func GetClusterRoleBinding() rbac.ClusterRoleBinding {
	return rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name: "docker-provider-manager-rolebinding",
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
			APIGroup: rbac.GroupName,
		},
		Subjects: []rbac.Subject{{
			Kind:      rbac.ServiceAccountKind,
			Name:      "default",
			Namespace: namespace,
		}},
	}
}
