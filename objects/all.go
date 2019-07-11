package objects

import (
	"k8s.io/apimachinery/pkg/runtime"
)

func GetManegementCluster(version, capiImage, capdImage string) ([]runtime.Object, error) {
	capiObjects, err := GetCAPI(version, capiImage)
	if err != nil {
		return []runtime.Object{}, err
	}

	namespaceObj := GetNamespace()
	statefulSet := GetStatefulSet(capdImage)
	clusterRole := GetClusterRole()
	clusterRoleBinding := GetClusterRoleBinding()

	return append(capiObjects,
		&namespaceObj,
		&statefulSet,
		&clusterRole,
		&clusterRoleBinding,
	), nil
}
