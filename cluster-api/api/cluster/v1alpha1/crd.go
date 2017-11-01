package v1alpha1

import (
	"fmt"
	"reflect"
	"time"

	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	ClustersCRDGroup   = "cluster-api.k8s.io"
	ClustersCRDPlural  = "clusters"
	ClustersCRDVersion = "v1alpha1"
	ClustersCRDName    = ClustersCRDPlural + "." + ClustersCRDGroup
)

var SchemeGroupVersion = schema.GroupVersion{Group: ClustersCRDGroup, Version: ClustersCRDVersion}

func CreateClustersCRD(clientset apiextensionsclient.Interface) (*extensionsv1.CustomResourceDefinition, error) {
	crd := &extensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClustersCRDName,
		},
		Spec: extensionsv1.CustomResourceDefinitionSpec{
			Group:   ClustersCRDGroup,
			Version: SchemeGroupVersion.Version,
			Scope:   extensionsv1.ClusterScoped,
			Names: extensionsv1.CustomResourceDefinitionNames{
				Plural: ClustersCRDPlural,
				Kind:   reflect.TypeOf(Cluster{}).Name(),
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		return nil, err
	}

	// wait for CRD being established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		crd, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ClustersCRDName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case extensionsv1.Established:
				if cond.Status == extensionsv1.ConditionTrue {
					return true, err
				}
			case extensionsv1.NamesAccepted:
				if cond.Status == extensionsv1.ConditionFalse {
					fmt.Printf("Name conflict: %v\n", cond.Reason)
				}
			}
		}
		return false, err
	})
	if err != nil {
		deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(ClustersCRDName, nil)
		if deleteErr != nil {
			return nil, errors.NewAggregate([]error{err, deleteErr})
		}
		return nil, err
	}
	return crd, nil
}
