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

package util

import (
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// GetObjectReferences accepts arguments in resource.apiversion/name or resource/name form (e.g. 'resource.apiversion/<resource_name>' or 'resource/<resource_name>') and returns an ObjectReference for each resource/name.
func GetObjectReferences(namespace string, args ...string) ([]corev1.ObjectReference, error) {
	var objRefs []corev1.ObjectReference
	if ok, err := hasCombinedTypeArgs(args); ok {
		if err != nil {
			return objRefs, err
		}
		for _, s := range args {
			ref, ok, err := convertToObjectRef(namespace, s)
			if err != nil {
				return objRefs, err
			}
			if ok {
				objRefs = append(objRefs, ref)
			}
		}
	} else {
		return objRefs, fmt.Errorf("arguments must be in resource.apiversion/name or resource/name format (e.g. deployment.v1/md-1 or deployment/md-1)")
	}
	return objRefs, nil
}

func hasCombinedTypeArgs(args []string) (bool, error) {
	hasSlash := 0
	for _, s := range args {
		if strings.Contains(s, "/") {
			hasSlash++
		}
	}
	switch {
	case hasSlash > 0 && hasSlash == len(args):
		return true, nil
	case hasSlash > 0 && hasSlash != len(args):
		baseCmd := "cmd"
		if len(os.Args) > 0 {
			baseCmdSlice := strings.Split(os.Args[0], "/")
			baseCmd = baseCmdSlice[len(baseCmdSlice)-1]
		}
		return true, fmt.Errorf("there is no need to specify a resource type as a separate argument when passing arguments in resource.apiversion/name or resource/name form (e.g. '%s get resource.apiversion/<resource_name>' instead of '%s get resource resource.apiversion/<resource_name>'", baseCmd, baseCmd)
	default:
		return false, nil
	}
}

// convertToObjectRef handles resource.apiversion/name or resource/name resource formats and returns an ObjectReference
// (empty or not), whether it successfully found one, and an error.
func convertToObjectRef(namespace, s string) (corev1.ObjectReference, bool, error) {
	if !strings.Contains(s, "/") {
		return corev1.ObjectReference{}, false, nil
	}
	seg := strings.Split(s, "/")
	if len(seg) != 2 {
		return corev1.ObjectReference{}, false, fmt.Errorf("arguments in resource.apiversion/name or resource/name form may not have more than one slash")
	}
	resourceVersion, name := seg[0], seg[1]
	if resourceVersion == "" || name == "" {
		return corev1.ObjectReference{}, false, fmt.Errorf("arguments in resource.apiversion/name or resource/name form must have a resource and name")
	}

	var resource, apiVersion string
	resourceVersionSeg := strings.Split(resourceVersion, ".")
	if len(resourceVersionSeg) == 2 {
		resource, apiVersion = resourceVersionSeg[0], resourceVersionSeg[1]
	} else if len(resourceVersionSeg) == 1 {
		resource = resourceVersionSeg[0]
	} else {
		return corev1.ObjectReference{}, false, fmt.Errorf("arguments in resource.apiversion/name or resource/name form must have a valid resource and optionally an apiversion separated by a dot")
	}
	if resource == "" {
		return corev1.ObjectReference{}, false, fmt.Errorf("resource in resource.apiversion/name or resource/name form must not be empty")
	}

	return corev1.ObjectReference{
		Kind:       resource,
		Name:       name,
		Namespace:  namespace,
		APIVersion: apiVersion,
	}, true, nil
}
