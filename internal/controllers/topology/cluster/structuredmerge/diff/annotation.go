/*
Copyright 2022 The Kubernetes Authors.

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

package diff

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// AddCurrentIntentAnnotation adds to the object a last applied intent annotation using the object itself as an intent.
// NOTE: this func is designed to be used in the server side helper, where the modified object that has
// been generated in compute desired state + patches is already trimmed down to a server side apply intent -
// the set of field the controller express an opinion on - by considering allowed and ignore paths.
func AddCurrentIntentAnnotation(obj *unstructured.Unstructured) error {
	currentIntentAnnotation, err := toCurrentIntentAnnotation(obj)
	if err != nil {
		return err
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[clusterv1.ClusterTopologyLastAppliedIntentAnnotation] = currentIntentAnnotation
	obj.SetAnnotations(annotations)

	return nil
}

func toCurrentIntentAnnotation(currentIntent *unstructured.Unstructured) (string, error) {
	// Converts to json.
	currentIntentJSON, err := json.Marshal(currentIntent.Object)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal current intent")
	}

	// gzip and base64 encode
	var currentIntentJSONGZIP bytes.Buffer
	zw := gzip.NewWriter(&currentIntentJSONGZIP)
	if _, err := zw.Write(currentIntentJSON); err != nil {
		return "", errors.Wrap(err, "failed to write current intent to gzip writer")
	}
	if err := zw.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close gzip writer for current intent")
	}
	currentIntentAnnotation := base64.StdEncoding.EncodeToString(currentIntentJSONGZIP.Bytes())
	return currentIntentAnnotation, nil
}

// GetPreviousIntent returns the previous intent stored in the object's the last applied annotation.
func GetPreviousIntent(obj client.Object) (*unstructured.Unstructured, error) {
	previousIntentAnnotation := obj.GetAnnotations()[clusterv1.ClusterTopologyLastAppliedIntentAnnotation]

	if previousIntentAnnotation == "" {
		return &unstructured.Unstructured{}, nil
	}

	previousIntentJSONGZIP, err := base64.StdEncoding.DecodeString(previousIntentAnnotation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode previous intent")
	}

	var previousIntentJSON bytes.Buffer
	zr, err := gzip.NewReader(bytes.NewReader(previousIntentJSONGZIP))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gzip reader forprevious intent")
	}

	if _, err := io.Copy(&previousIntentJSON, zr); err != nil { //nolint:gosec
		return nil, errors.Wrap(err, "failed to copy from gzip reader")
	}

	if err := zr.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to close gzip reader for managed fields")
	}

	previousIntentMap := make(map[string]interface{})
	if err := json.Unmarshal(previousIntentJSON.Bytes(), &previousIntentMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal managed fields")
	}

	return &unstructured.Unstructured{Object: previousIntentMap}, nil
}
