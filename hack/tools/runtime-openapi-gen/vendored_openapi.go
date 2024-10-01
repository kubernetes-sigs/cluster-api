/*
Copyright 2021 The Kubernetes Authors.

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

package main

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// GetOpenAPIDefinitions returns OpenAPIDefinitions for external types.
// We need them when we want to generate OpenAPI for a type embedding one of those external types.
// Those OpenAPIDefinitions are all vendored from: https://github.com/kubernetes/kubernetes/blob/master/pkg/generated/openapi/zz_generated.openapi.go
// Prior art: https://github.com/kubernetes-sigs/controller-tools/blob/master/pkg/crd/known_types.go.
func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"k8s.io/api/core/v1.ObjectReference":                            schema_k8sio_api_core_v1_ObjectReference(ref),
		"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1.JSON": schema_pkg_apis_apiextensions_v1_JSON(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.Duration":                 schema_pkg_apis_meta_v1_Duration(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.FieldsV1":                 schema_pkg_apis_meta_v1_FieldsV1(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.ManagedFieldsEntry":       schema_pkg_apis_meta_v1_ManagedFieldsEntry(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta":               schema_pkg_apis_meta_v1_ObjectMeta(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.OwnerReference":           schema_pkg_apis_meta_v1_OwnerReference(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.Time":                     schema_pkg_apis_meta_v1_Time(ref),
		"k8s.io/apimachinery/pkg/runtime.RawExtension":                  schema_k8sio_apimachinery_pkg_runtime_RawExtension(ref),
		"k8s.io/apimachinery/pkg/util/intstr.IntOrString":               schema_apimachinery_pkg_util_intstr_IntOrString(ref),
		"k8s.io/apimachinery/pkg/apis/meta/v1.Condition":                schema_pkg_apis_meta_v1_Condition(ref),
	}
}

func schema_pkg_apis_apiextensions_v1_JSON(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "JSON represents any valid JSON value. These types are supported: bool, int64, float64, string, []interface{}, map[string]interface{} and nil.",
				Type:        apiextensionsv1.JSON{}.OpenAPISchemaType(),
				Format:      apiextensionsv1.JSON{}.OpenAPISchemaFormat(),
			},
		},
	}
}

func schema_pkg_apis_meta_v1_Duration(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Duration is a wrapper around time.Duration which supports correct marshaling to YAML and JSON. In particular, it marshals into strings, which can be used as map keys in json.",
				Type:        metav1.Duration{}.OpenAPISchemaType(),
				Format:      metav1.Duration{}.OpenAPISchemaFormat(),
			},
		},
	}
}

func schema_pkg_apis_meta_v1_FieldsV1(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "FieldsV1 stores a set of fields in a data structure like a Trie, in JSON format.\n\nEach key is either a '.' representing the field itself, and will always map to an empty set, or a string representing a sub-field or item. The string will follow one of these four formats: 'f:<name>', where <name> is the name of a field in a struct, or key in a map 'v:<value>', where <value> is the exact json formatted value of a list item 'i:<index>', where <index> is position of a item in a list 'k:<keys>', where <keys> is a map of  a list item's key fields to their unique values If a key maps to an empty Fields value, the field that key represents is part of the set.\n\nThe exact format is defined in sigs.k8s.io/structured-merge-diff",
				Type:        []string{"object"},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_ManagedFieldsEntry(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ManagedFieldsEntry is a workflow-id, a FieldSet and the group version of the resource that the fieldset applies to.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"manager": {
						SchemaProps: spec.SchemaProps{
							Description: "Manager is an identifier of the workflow managing these fields.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"operation": {
						SchemaProps: spec.SchemaProps{
							Description: "Operation is the type of operation which lead to this ManagedFieldsEntry being created. The only valid values for this field are 'Apply' and 'Update'.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the version of this resource that this field set applies to. The format is \"group/version\" just like the top-level APIVersion field. It is necessary to track the version of a field set because it cannot be automatically converted.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"time": {
						SchemaProps: spec.SchemaProps{
							Description: "Time is timestamp of when these fields were set. It should always be empty if Operation is 'Apply'",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
					"fieldsType": {
						SchemaProps: spec.SchemaProps{
							Description: "FieldsType is the discriminator for the different fields format and version. There is currently only one possible value: \"FieldsV1\"",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"fieldsV1": {
						SchemaProps: spec.SchemaProps{
							Description: "FieldsV1 holds the first JSON version format as described in the \"FieldsV1\" type.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.FieldsV1"),
						},
					},
					"subresource": {
						SchemaProps: spec.SchemaProps{
							Description: "Subresource is the name of the subresource used to update that object, or empty string if the object was updated through the main resource. The value of this field is used to distinguish between managers, even if they share the same name. For example, a status update will be distinct from a regular update using the same manager name. Note that the APIVersion field is not related to the Subresource field and it always corresponds to the version of the main resource.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.FieldsV1", "k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}

func schema_pkg_apis_meta_v1_ObjectMeta(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ObjectMeta is metadata that all persisted resources must have, which includes all objects users must create.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"generateName": {
						SchemaProps: spec.SchemaProps{
							Description: "GenerateName is an optional prefix, used by the server, to generate a unique name ONLY IF the Name field has not been provided. If this field is used, the name returned to the client will be different than the name passed. This value will also be combined with a unique suffix. The provided value has the same validation rules as the Name field, and may be truncated by the length of the suffix required to make the value unique on the server.\n\nIf this field is specified and the generated name exists, the server will NOT return a 409 - instead, it will either return 201 Created or 500 with Reason ServerTimeout indicating a unique name could not be found in the time allotted, and the client should retry (optionally after the time indicated in the Retry-After header).\n\nApplied only if Name is not specified. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#idempotency",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"namespace": {
						SchemaProps: spec.SchemaProps{
							Description: "Namespace defines the space within which each name must be unique. An empty namespace is equivalent to the \"default\" namespace, but \"default\" is the canonical representation. Not all objects are required to be scoped to a namespace - the value of this field for those objects will be empty.\n\nMust be a DNS_LABEL. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/namespaces",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"selfLink": {
						SchemaProps: spec.SchemaProps{
							Description: "SelfLink is a URL representing this object. Populated by the system. Read-only.\n\nDEPRECATED Kubernetes will stop propagating this field in 1.20 release and the field is planned to be removed in 1.21 release.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID is the unique in time and space value for this object. It is typically generated by the server on successful creation of a resource and is not allowed to change on PUT operations.\n\nPopulated by the system. Read-only. More info: http://kubernetes.io/docs/user-guide/identifiers#uids",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"resourceVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "An opaque value that represents the internal version of this object that can be used by clients to determine when objects have changed. May be used for optimistic concurrency, change detection, and the watch operation on a resource or set of resources. Clients must treat these values as opaque and passed unmodified back to the server. They may only be valid for a particular resource or set of resources.\n\nPopulated by the system. Read-only. Value must be treated as opaque by clients. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"generation": {
						SchemaProps: spec.SchemaProps{
							Description: "A sequence number representing a specific generation of the desired state. Populated by the system. Read-only.",
							Type:        []string{"integer"},
							Format:      "int64",
						},
					},
					"creationTimestamp": {
						SchemaProps: spec.SchemaProps{
							Description: "CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC.\n\nPopulated by the system. Read-only. Null for lists. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata",
							Default:     map[string]interface{}{},
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
					"deletionTimestamp": {
						SchemaProps: spec.SchemaProps{
							Description: "DeletionTimestamp is RFC 3339 date and time at which this resource will be deleted. This field is set by the server when a graceful deletion is requested by the user, and is not directly settable by a client. The resource is expected to be deleted (no longer visible from resource lists, and not reachable by name) after the time in this field, once the finalizers list is empty. As long as the finalizers list contains items, deletion is blocked. Once the deletionTimestamp is set, this value may not be unset or be set further into the future, although it may be shortened or the resource may be deleted prior to this time. For example, a user may request that a pod is deleted in 30 seconds. The Kubelet will react by sending a graceful termination signal to the containers in the pod. After that 30 seconds, the Kubelet will send a hard termination signal (SIGKILL) to the container and after cleanup, remove the pod from the API. In the presence of network partitions, this object may still exist after this timestamp, until an administrator or automated process can determine the resource is fully terminated. If not set, graceful deletion of the object has not been requested.\n\nPopulated by the system when a graceful deletion is requested. Read-only. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
					"deletionGracePeriodSeconds": {
						SchemaProps: spec.SchemaProps{
							Description: "Number of seconds allowed for this object to gracefully terminate before it will be removed from the system. Only set when deletionTimestamp is also set. May only be shortened. Read-only.",
							Type:        []string{"integer"},
							Format:      "int64",
						},
					},
					"labels": {
						SchemaProps: spec.SchemaProps{
							Description: "Map of string keys and values that can be used to organize and categorize (scope and select) objects. May match selectors of replication controllers and services. More info: http://kubernetes.io/docs/user-guide/labels",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"annotations": {
						SchemaProps: spec.SchemaProps{
							Description: "Annotations is an unstructured key value map stored with a resource that may be set by external tools to store and retrieve arbitrary metadata. They are not queryable and should be preserved when modifying objects. More info: http://kubernetes.io/docs/user-guide/annotations",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"ownerReferences": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-patch-merge-key": "uid",
								"x-kubernetes-patch-strategy":  "merge",
							},
						},
						SchemaProps: spec.SchemaProps{
							Description: "List of objects depended by this object. If ALL objects in the list have been deleted, this object will be garbage collected. If this object is managed by a controller, then an entry in this list will point to this controller, with the controller field set to true. There cannot be more than one managing controller.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.OwnerReference"),
									},
								},
							},
						},
					},
					"finalizers": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-patch-strategy": "merge",
							},
						},
						SchemaProps: spec.SchemaProps{
							Description: "Must be empty before the object is deleted from the registry. Each entry is an identifier for the responsible component that will remove the entry from the list. If the deletionTimestamp of the object is non-nil, entries in this list can only be removed. Finalizers may be processed and removed in any order.  Order is NOT enforced because it introduces significant risk of stuck finalizers. finalizers is a shared field, any actor with permission can reorder it. If the finalizer list is processed in order, then this can lead to a situation in which the component responsible for the first finalizer in the list is waiting for a signal (field value, external system, or other) produced by a component responsible for a finalizer later in the list, resulting in a deadlock. Without enforced ordering finalizers are free to order amongst themselves and are not vulnerable to ordering changes in the list.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"clusterName": {
						SchemaProps: spec.SchemaProps{
							Description: "The name of the cluster which the object belongs to. This is used to distinguish resources with same name and namespace in different clusters. This field is not set anywhere right now and apiserver is going to ignore it if set in create or update request.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"managedFields": {
						SchemaProps: spec.SchemaProps{
							Description: "ManagedFields maps workflow-id and version to the set of fields that are managed by that workflow. This is mostly for internal housekeeping, and users typically shouldn't need to set or understand this field. A workflow can be the user's name, a controller's name, or the name of a specific apply path like \"ci-cd\". The set of fields is always in the version that the workflow used when modifying the object.",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]interface{}{},
										Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ManagedFieldsEntry"),
									},
								},
							},
						},
					},
				},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.ManagedFieldsEntry", "k8s.io/apimachinery/pkg/apis/meta/v1.OwnerReference", "k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}

func schema_pkg_apis_meta_v1_OwnerReference(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "OwnerReference contains enough information to let you identify an owning object. An owning object must be in the same namespace as the dependent, or be cluster-scoped, so there is no namespace field.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "API version of the referent.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind of the referent. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name of the referent. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID of the referent. More info: http://kubernetes.io/docs/user-guide/identifiers#uids",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"controller": {
						SchemaProps: spec.SchemaProps{
							Description: "If true, this reference points to the managing controller.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"blockOwnerDeletion": {
						SchemaProps: spec.SchemaProps{
							Description: "If true, AND if the owner has the \"foregroundDeletion\" finalizer, then the owner cannot be deleted from the key-value store until this reference is removed. Defaults to false. To set this field, a user needs \"delete\" permission of the owner, otherwise 422 (Unprocessable Entity) will be returned.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
				},
				Required: []string{"apiVersion", "kind", "name", "uid"},
			},
			VendorExtensible: spec.VendorExtensible{
				Extensions: spec.Extensions{
					"x-kubernetes-map-type": "atomic",
				},
			},
		},
	}
}

func schema_pkg_apis_meta_v1_Time(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Time is a wrapper around time.Time which supports correct marshaling to YAML and JSON.  Wrappers are provided for many of the factory methods that the time package offers.",
				Type:        metav1.Time{}.OpenAPISchemaType(),
				Format:      metav1.Time{}.OpenAPISchemaFormat(),
			},
		},
	}
}

func schema_k8sio_apimachinery_pkg_runtime_RawExtension(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "RawExtension is used to hold extensions in external versions.\n\nTo use this, make a field which has RawExtension as its type in your external, versioned struct, and Object in your internal struct. You also need to register your various plugin types.\n\n// Internal package: type MyAPIObject struct {\n\truntime.TypeMeta `json:\",inline\"`\n\tMyPlugin runtime.Object `json:\"myPlugin\"`\n} type PluginA struct {\n\tAOption string `json:\"aOption\"`\n}\n\n// External package: type MyAPIObject struct {\n\truntime.TypeMeta `json:\",inline\"`\n\tMyPlugin runtime.RawExtension `json:\"myPlugin\"`\n} type PluginA struct {\n\tAOption string `json:\"aOption\"`\n}\n\n// On the wire, the JSON will look something like this: {\n\t\"kind\":\"MyAPIObject\",\n\t\"apiVersion\":\"v1\",\n\t\"myPlugin\": {\n\t\t\"kind\":\"PluginA\",\n\t\t\"aOption\":\"foo\",\n\t},\n}\n\nSo what happens? Decode first uses json or yaml to unmarshal the serialized data into your external MyAPIObject. That causes the raw JSON to be stored, but not unpacked. The next step is to copy (using pkg/conversion) into the internal struct. The runtime package's DefaultScheme has conversion functions installed which will unpack the JSON stored in RawExtension, turning it into the correct object type, and storing it in the Object. (TODO: In the case where the object is of an unknown type, a runtime.Unknown object will be created and stored.)",
				Type:        []string{"object"},
			},
		},
	}
}

func schema_k8sio_api_core_v1_ObjectReference(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ObjectReference contains enough information to let you inspect or modify the referred object.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind of the referent. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"namespace": {
						SchemaProps: spec.SchemaProps{
							Description: "Namespace of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"uid": {
						SchemaProps: spec.SchemaProps{
							Description: "UID of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "API version of the referent.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"resourceVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "Specific resourceVersion to which this reference is made, if any. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"fieldPath": {
						SchemaProps: spec.SchemaProps{
							Description: "If referring to a piece of an object instead of an entire object, this string should contain a valid JSON/Go field access statement, such as desiredState.manifest.containers[2]. For example, if the object reference is to a container within a pod, this would take on a value like: \"spec.containers{name}\" (where \"name\" refers to the name of the container that triggered the event) or if no container name is specified \"spec.containers[2]\" (container with index 2 in this pod). This syntax is chosen only to have some well-defined way of referencing a part of an object.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
			},
			VendorExtensible: spec.VendorExtensible{
				Extensions: spec.Extensions{
					"x-kubernetes-map-type": "atomic",
				},
			},
		},
	}
}

func schema_apimachinery_pkg_util_intstr_IntOrString(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.EmbedOpenAPIDefinitionIntoV2Extension(common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "IntOrString is a type that can hold an int32 or a string.  When used in JSON or YAML marshalling and unmarshalling, it produces or consumes the inner type.  This allows you to have, for example, a JSON field that can accept a name or number.",
				OneOf:       common.GenerateOpenAPIV3OneOfSchema(intstr.IntOrString{}.OpenAPIV3OneOfTypes()),
				Format:      intstr.IntOrString{}.OpenAPISchemaFormat(),
			},
		},
	}, common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "IntOrString is a type that can hold an int32 or a string.  When used in JSON or YAML marshalling and unmarshalling, it produces or consumes the inner type.  This allows you to have, for example, a JSON field that can accept a name or number.",
				Type:        intstr.IntOrString{}.OpenAPISchemaType(),
				Format:      intstr.IntOrString{}.OpenAPISchemaFormat(),
			},
		},
	})
}

func schema_pkg_apis_meta_v1_Condition(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Condition contains details for one aspect of the current state of this API Resource.",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"type": {
						SchemaProps: spec.SchemaProps{
							Description: "type of condition in CamelCase or in foo.example.com/CamelCase.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Description: "status of the condition, one of True, False, Unknown.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"observedGeneration": {
						SchemaProps: spec.SchemaProps{
							Description: "observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance.",
							Type:        []string{"integer"},
							Format:      "int64",
						},
					},
					"lastTransitionTime": {
						SchemaProps: spec.SchemaProps{
							Description: "lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
					"reason": {
						SchemaProps: spec.SchemaProps{
							Description: "reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"message": {
						SchemaProps: spec.SchemaProps{
							Description: "message is a human readable message indicating details about the transition. This may be an empty string.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
				Required: []string{"type", "status", "lastTransitionTime", "reason", "message"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}
