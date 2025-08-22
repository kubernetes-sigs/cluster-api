/*
Copyright 2023 The Kubernetes Authors.

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

package api

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

var (
	// apiVersions is the value returned by /api discovery call.
	// Note: This must contain all APIs required by CAPI.
	apiVersions = &metav1.APIVersions{
		Versions: []string{"v1"},
	}

	// corev1APIResourceList is the value returned by /api/v1 discovery call.
	// Note: This must contain all APIs required by CAPI.
	corev1APIResourceList = &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{
				Name:         "configmaps",
				SingularName: "",
				Namespaced:   true,
				Kind:         "ConfigMap",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"cm",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "endpoints",
				SingularName: "",
				Namespaced:   true,
				Kind:         "Endpoints",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"ep",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "nodes",
				SingularName: "",
				Namespaced:   false,
				Kind:         "Node",
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
				ShortNames: []string{
					"no",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "pods",
				SingularName: "",
				Namespaced:   true,
				Kind:         "Pod",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"po",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "secrets",
				SingularName: "",
				Namespaced:   true,
				Kind:         "Secret",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "services",
				SingularName: "",
				Namespaced:   true,
				Kind:         "Service",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"svc",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "namespaces",
				SingularName: "namespace",
				Namespaced:   false,
				Kind:         "Namespace",
				Verbs: []string{
					"create",
					"delete",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"ns",
				},
				StorageVersionHash: "",
			},
		},
	}

	// apiGroupList is the value returned by /apis discovery call.
	// Note: This must contain all APIs required by CAPI.
	apiGroupList = &metav1.APIGroupList{
		Groups: []metav1.APIGroup{
			{
				Name: "rbac.authorization.k8s.io",
				Versions: []metav1.GroupVersionForDiscovery{
					{
						GroupVersion: "rbac.authorization.k8s.io/v1",
						Version:      "v1",
					},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{
					GroupVersion: "rbac.authorization.k8s.io/v1",
					Version:      "v1",
				},
			},
			{
				Name: "apps",
				Versions: []metav1.GroupVersionForDiscovery{
					{
						GroupVersion: "apps/v1",
						Version:      "v1",
					},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{
					GroupVersion: "apps/v1",
					Version:      "v1",
				},
			},
		},
	}

	// rbacv1APIResourceList is the value returned by /apis/rbac.authorization.k8s.io/v1 discovery call.
	// Note: This must contain all APIs required by CAPI.
	rbacv1APIResourceList = &metav1.APIResourceList{
		GroupVersion: "rbac.authorization.k8s.io/v1",
		APIResources: []metav1.APIResource{
			{
				Name:         "clusterrolebindings",
				SingularName: "",
				Namespaced:   false,
				Kind:         "ClusterRoleBinding",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "clusterroles",
				SingularName: "",
				Namespaced:   false,
				Kind:         "ClusterRole",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "rolebindings",
				SingularName: "",
				Namespaced:   true,
				Kind:         "RoleBinding",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "roles",
				SingularName: "",
				Namespaced:   true,
				Kind:         "Role",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				StorageVersionHash: "",
			},
		},
	}

	// appsV1ResourceList is the value returned by /apis/apps/v1 discovery call.
	// Note: This must contain all APIs required by CAPI.
	appsV1ResourceList = &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{
			{
				Name:         "daemonsets",
				SingularName: "daemonset",
				Namespaced:   true,
				Kind:         "DaemonSet",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"ds",
				},
				StorageVersionHash: "",
			},
			{
				Name:         "deployments",
				SingularName: "deployment",
				Namespaced:   true,
				Kind:         "Deployment",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames: []string{
					"deploy",
				},
				StorageVersionHash: "",
			},
		},
	}

	// storageV1ResourceList is the value returned by /apis/storage.k8s.io/v1 discovery call.
	// Note: This must contain all APIs required by CAPI.
	storageV1ResourceList = &metav1.APIResourceList{
		GroupVersion: "storage.k8s.io/v1",
		APIResources: []metav1.APIResource{
			{
				Name:         "volumeattachments",
				SingularName: "volumeattachment",
				Namespaced:   false,
				Kind:         "VolumeAttachment",
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
				ShortNames:         []string{},
				StorageVersionHash: "",
			},
		},
	}
)
