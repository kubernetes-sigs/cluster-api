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

	// apiVersions is the value returned by /api/v1 discovery call.
	// Note: This must contain all APIs required by CAPI.
	corev1APIResourceList = &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
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
		},
	}

	// apiVersions is the value returned by /apis discovery call.
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
		},
	}

	// apiVersions is the value returned by /apis/rbac.authorization.k8s.io/v1  discovery call.
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
)
