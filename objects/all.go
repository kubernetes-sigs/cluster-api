package objects

import "k8s.io/apimachinery/pkg/runtime"

func GetAll(capdImage string) []runtime.Object {
	namespaceObj := GetNamespace()
	statefulSet := GetStatefulSet(capdImage)
	clusterRole := GetClusterRole()
	clusterRoleBinding := GetClusterRoleBinding()

	return []runtime.Object{
		&namespaceObj,
		&statefulSet,
		&clusterRole,
		&clusterRoleBinding,
	}
}
