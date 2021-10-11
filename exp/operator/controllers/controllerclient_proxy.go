package controllers

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

type controllerProxy struct {
	ctrlClient client.Client
	ctrlConfig *rest.Config
}

var _ cluster.Proxy = &controllerProxy{}

func (k *controllerProxy) CurrentNamespace() (string, error)           { return "default", nil }
func (k *controllerProxy) ValidateKubernetesVersion() error            { return nil }
func (k *controllerProxy) GetConfig() (*rest.Config, error)            { return k.ctrlConfig, nil }
func (k *controllerProxy) NewClient() (client.Client, error)           { return k.ctrlClient, nil }
func (k *controllerProxy) GetContexts(prefix string) ([]string, error) { return nil, nil }
func (k *controllerProxy) CheckClusterAvailable() error                { return nil }

// GetResourceNames returns the list of resource names which begin with prefix.
func (k *controllerProxy) GetResourceNames(groupVersion, kind string, options []client.ListOption, prefix string) ([]string, error) {
	objList, err := listObjByGVK(k.ctrlClient, groupVersion, kind, options)
	if err != nil {
		return nil, err
	}

	var comps []string
	for _, item := range objList.Items {
		name := item.GetName()

		if strings.HasPrefix(name, prefix) {
			comps = append(comps, name)
		}
	}

	return comps, nil
}

// ListResources return only RBAC and Deployoments as this is used by:
// - from Delete return just RBAC, we don't delete CRDs, Namespaces and resources in the namespace.
// - from certmanager just return the resource that has a version annontation.
func (k *controllerProxy) ListResources(labels map[string]string, namespaces ...string) ([]unstructured.Unstructured, error) {
	resourceList := []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Kind: "Secret", Namespaced: true},
				{Kind: "ConfigMap", Namespaced: true},
				{Kind: "Service", Namespaced: true},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Kind: "DaemonSet", Namespaced: true},
				{Kind: "Deployment", Namespaced: true},
			},
		},
		{
			GroupVersion: "rbac.authorization.k8s.io/v1",
			APIResources: []metav1.APIResource{
				{Kind: "ClusterRoleBinding"},
				{Kind: "ClusterRole"},
				{Kind: "RoleBinding", Namespaced: true},
				{Kind: "Role", Namespaced: true},
			},
		},
		{
			GroupVersion: "admissionregistration.k8s.io/v1",
			APIResources: []metav1.APIResource{
				{Kind: "ValidatingWebhookConfiguration", Namespaced: true},
				{Kind: "MutatingWebhookConfiguration", Namespaced: true},
			},
		},
		{
			GroupVersion: "cert-manager.io/v1",
			APIResources: []metav1.APIResource{
				{Kind: "Certificate", Namespaced: true},
				{Kind: "CertificateRequest", Namespaced: true},
				{Kind: "Issuer", Namespaced: true},
			},
		},
	}

	var ret []unstructured.Unstructured
	for _, resourceGroup := range resourceList {
		for _, resourceKind := range resourceGroup.APIResources {
			if resourceKind.Namespaced {
				for _, namespace := range namespaces {
					objList, err := listObjByGVK(k.ctrlClient, resourceGroup.GroupVersion, resourceKind.Kind, []client.ListOption{client.MatchingLabels(labels), client.InNamespace(namespace)})
					if err != nil {
						return nil, err
					}
					klog.V(3).InfoS("listed", "kind", resourceKind.Kind, "count", len(objList.Items))
					ret = append(ret, objList.Items...)
				}
			} else {
				objList, err := listObjByGVK(k.ctrlClient, resourceGroup.GroupVersion, resourceKind.Kind, []client.ListOption{client.MatchingLabels(labels)})
				if err != nil {
					return nil, err
				}
				klog.V(3).InfoS("listed", "kind", resourceKind.Kind, "count", len(objList.Items))
				ret = append(ret, objList.Items...)
			}
		}
	}
	return ret, nil
}

func listObjByGVK(c client.Client, groupVersion, kind string, options []client.ListOption) (*unstructured.UnstructuredList, error) {
	ctx := context.TODO()
	objList := new(unstructured.UnstructuredList)
	objList.SetAPIVersion(groupVersion)
	objList.SetKind(kind)

	if err := c.List(ctx, objList, options...); err != nil {
		// not all clusters (including unit tests) will have cert-manager CRDs.
		if _, ok := err.(*meta.NoKindMatchError); !ok {
			return nil, errors.Wrapf(err, "failed to list objects for the %q GroupVersionKind", objList.GroupVersionKind())
		}
	}
	return objList, nil
}
