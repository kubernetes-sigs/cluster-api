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

package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
)

const (
	// injectCAFromSecretAnnotationField is used by the Extension controller for indexing ExtensionConfigs
	// which have the InjectCAFromSecretAnnotation set.
	injectCAFromSecretAnnotationField = "metadata.annotations[" + runtimev1.InjectCAFromSecretAnnotation + "]"
)

// indexByExtensionInjectCAFromSecretName adds the index by InjectCAFromSecretAnnotation to the
// managers cache.
func indexByExtensionInjectCAFromSecretName(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &runtimev1.ExtensionConfig{},
		injectCAFromSecretAnnotationField,
		extensionConfigByInjectCAFromSecretName,
	); err != nil {
		return errors.Wrap(err, "error setting index field for InjectCAFromSecretAnnotation")
	}
	return nil
}

func extensionConfigByInjectCAFromSecretName(o client.Object) []string {
	extensionConfig, ok := o.(*runtimev1.ExtensionConfig)
	if !ok {
		panic(fmt.Sprintf("Expected ExtensionConfig but got a %T", o))
	}
	if value, ok := extensionConfig.Annotations[runtimev1.InjectCAFromSecretAnnotation]; ok {
		return []string{value}
	}
	return nil
}
