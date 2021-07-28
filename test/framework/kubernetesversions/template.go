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

// Package kubernetesversions implements kubernetes version functions.
package kubernetesversions

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/yaml"
)

var (
	//go:embed data/debian_injection_script.envsubst.sh.tpl
	debianInjectionScriptBytes string

	debianInjectionScriptTemplate = template.Must(template.New("").Parse(debianInjectionScriptBytes))

	//go:embed data/kustomization.yaml
	kustomizationYAMLBytes string
)

type GenerateCIArtifactsInjectedTemplateForDebianInput struct {
	// ArtifactsDirectory is where conformance suite output will go. Defaults to _artifacts
	ArtifactsDirectory string
	// SourceTemplate is an input YAML clusterctl template which is to have
	// the CI artifact script injection
	SourceTemplate []byte
	// PlatformKustomization is an SMP (strategic-merge-style) patch for adding
	// platform specific kustomizations required for use with CI, such as
	// referencing a specific image
	PlatformKustomization []byte
	// KubeadmConfigTemplateName is the name of the KubeadmConfigTemplate resource
	// that needs to have the Debian install script injected. Defaults to "${CLUSTER_NAME}-md-0".
	KubeadmConfigTemplateName string
	// KubeadmControlPlaneName is the name of the KubeadmControlPlane resource
	// that needs to have the Debian install script injected. Defaults to "${CLUSTER_NAME}-control-plane".
	KubeadmControlPlaneName string
	// KubeadmConfigName is the name of a KubeadmConfig that needs kustomizing. To be used in conjunction with MachinePools. Optional.
	KubeadmConfigName string
}

// GenerateCIArtifactsInjectedTemplateForDebian takes a source clusterctl template
// and a platform-specific Kustomize SMP patch and injects a bash script to download
// and install the debian packages for the given Kubernetes version, returning the
// location of the outputted file.
func GenerateCIArtifactsInjectedTemplateForDebian(input GenerateCIArtifactsInjectedTemplateForDebianInput) (string, error) {
	if input.SourceTemplate == nil {
		return "", errors.New("SourceTemplate must be provided")
	}
	input.ArtifactsDirectory = framework.ResolveArtifactsDirectory(input.ArtifactsDirectory)
	if input.KubeadmConfigTemplateName == "" {
		input.KubeadmConfigTemplateName = "${CLUSTER_NAME}-md-0"
	}
	if input.KubeadmControlPlaneName == "" {
		input.KubeadmControlPlaneName = "${CLUSTER_NAME}-control-plane"
	}
	templateDir := path.Join(input.ArtifactsDirectory, "templates")
	overlayDir := path.Join(input.ArtifactsDirectory, "overlay")

	if err := os.MkdirAll(templateDir, 0o750); err != nil {
		return "", err
	}
	if err := os.MkdirAll(overlayDir, 0o750); err != nil {
		return "", err
	}

	kustomizedTemplate := path.Join(templateDir, "cluster-template-conformance-ci-artifacts.yaml")

	if err := ioutil.WriteFile(path.Join(overlayDir, "kustomization.yaml"), []byte(kustomizationYAMLBytes), 0o600); err != nil {
		return "", err
	}

	var debianInjectionScriptControlPlaneBytes bytes.Buffer
	if err := debianInjectionScriptTemplate.Execute(&debianInjectionScriptControlPlaneBytes, map[string]bool{"IsControlPlaneMachine": true}); err != nil {
		return "", err
	}
	patch, err := generateInjectScriptJSONPatch(input.SourceTemplate, "KubeadmControlPlane", input.KubeadmControlPlaneName, "/spec/kubeadmConfigSpec", "/usr/local/bin/ci-artifacts.sh", debianInjectionScriptControlPlaneBytes.String())
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(overlayDir, "kubeadmcontrolplane-patch.yaml"), patch, 0o600); err != nil {
		return "", err
	}

	var debianInjectionScriptWorkerBytes bytes.Buffer
	if err := debianInjectionScriptTemplate.Execute(&debianInjectionScriptWorkerBytes, map[string]bool{"IsControlPlaneMachine": false}); err != nil {
		return "", err
	}
	patch, err = generateInjectScriptJSONPatch(input.SourceTemplate, "KubeadmConfigTemplate", input.KubeadmConfigTemplateName, "/spec/template/spec", "/usr/local/bin/ci-artifacts.sh", debianInjectionScriptWorkerBytes.String())
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(overlayDir, "kubeadmconfigtemplate-patch.yaml"), patch, 0o600); err != nil {
		return "", err
	}

	if err := os.WriteFile(path.Join(overlayDir, "ci-artifacts-source-template.yaml"), input.SourceTemplate, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(overlayDir, "platform-kustomization.yaml"), input.PlatformKustomization, 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("kustomize", "build", overlayDir) //nolint:gosec // We don't care about command injection here.
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(kustomizedTemplate, data, 0o600); err != nil {
		return "", err
	}
	return kustomizedTemplate, nil
}

type jsonPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// generateInjectScriptJSONPatch generates a JSON patch which injects a script
// * objectKind: is the kind of the object we want to inject the script into
// * objectName: is the name of the object we want to inject the script into
// * jsonPatchPathPrefix: is the prefix of the 'files' and `preKubeadmCommands` arrays where we append the script
// * scriptPath: is the path where the script will be stored at
// * scriptContent: content of the script.
func generateInjectScriptJSONPatch(sourceTemplate []byte, objectKind, objectName, jsonPatchPathPrefix, scriptPath, scriptContent string) ([]byte, error) {
	filesPathExists, preKubeadmCommandsPathExists, err := checkIfArraysAlreadyExist(sourceTemplate, objectKind, objectName, jsonPatchPathPrefix)
	if err != nil {
		return nil, err
	}

	var patches []jsonPatch
	if !filesPathExists {
		patches = append(patches, jsonPatch{
			Op:    "add",
			Path:  fmt.Sprintf("%s/files", jsonPatchPathPrefix),
			Value: []interface{}{},
		})
	}
	patches = append(patches, jsonPatch{
		Op:   "add",
		Path: fmt.Sprintf("%s/files/-", jsonPatchPathPrefix),
		Value: map[string]string{
			"content":     scriptContent,
			"owner":       "root:root",
			"path":        scriptPath,
			"permissions": "0750",
		},
	})
	if !preKubeadmCommandsPathExists {
		patches = append(patches, jsonPatch{
			Op:    "add",
			Path:  fmt.Sprintf("%s/preKubeadmCommands", jsonPatchPathPrefix),
			Value: []string{},
		})
	}
	patches = append(patches, jsonPatch{
		Op:    "add",
		Path:  fmt.Sprintf("%s/preKubeadmCommands/-", jsonPatchPathPrefix),
		Value: scriptPath,
	})

	return yaml.Marshal(patches)
}

// checkIfArraysAlreadyExist check is the 'files' and 'preKubeadmCommands' arrays already exist below jsonPatchPathPrefix.
func checkIfArraysAlreadyExist(sourceTemplate []byte, objectKind, objectName, jsonPatchPathPrefix string) (bool, bool, error) {
	yamlDocs := strings.Split(string(sourceTemplate), "---")
	for _, yamlDoc := range yamlDocs {
		if yamlDoc == "" {
			continue
		}
		var obj unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(yamlDoc), &obj); err != nil {
			return false, false, err
		}

		if obj.GetKind() != objectKind {
			continue
		}
		if obj.GetName() != objectName {
			continue
		}

		pathSplit := strings.Split(strings.TrimPrefix(jsonPatchPathPrefix, "/"), "/")
		filesPath := append(pathSplit, "files")
		preKubeadmCommandsPath := append(pathSplit, "preKubeadmCommands")
		_, filesPathExists, err := unstructured.NestedFieldCopy(obj.Object, filesPath...)
		if err != nil {
			return false, false, err
		}
		_, preKubeadmCommandsPathExists, err := unstructured.NestedFieldCopy(obj.Object, preKubeadmCommandsPath...)
		if err != nil {
			return false, false, err
		}
		return filesPathExists, preKubeadmCommandsPathExists, nil
	}
	return false, false, fmt.Errorf("could not find document with kind %q and name %q", objectKind, objectName)
}
