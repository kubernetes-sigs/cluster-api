package objects

import "k8s.io/apimachinery/pkg/runtime"

func GetAll(capdImage string) []runtime.Object {
	statefulSet := GetStatefulSet(capdImage)

	return []runtime.Object{
		&Namespace,
		&statefulSet,
		&ClusterRole,
		&ClusterRoleBinding,
	}
}
