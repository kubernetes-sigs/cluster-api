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
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/yaml"
)

const yamlSeparator = "\n---\n"

var (
	//go:embed data/kustomization.yaml
	kustomizationYamlBytes []byte

	//go:embed data/debian_injection_script.envsubst.sh
	debianInjectionScriptBytes string
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

	if err := os.WriteFile(path.Join(overlayDir, "kustomization.yaml"), kustomizationYamlBytes, 0o600); err != nil {
		return "", err
	}

	kustomizeVersions, err := generateKustomizeVersionsYaml(input.KubeadmControlPlaneName, input.KubeadmConfigTemplateName, input.KubeadmConfigName)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path.Join(overlayDir, "kustomizeversions.yaml"), kustomizeVersions, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(overlayDir, "ci-artifacts-source-template.yaml"), input.SourceTemplate, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(path.Join(overlayDir, "platform-kustomization.yaml"), input.PlatformKustomization, 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("kustomize", "build", overlayDir)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(kustomizedTemplate, data, 0o600); err != nil {
		return "", err
	}
	return kustomizedTemplate, nil
}

func generateKustomizeVersionsYaml(kcpName, kubeadmTemplateName, kubeadmConfigName string) ([]byte, error) {
	kcp := generateKubeadmControlPlane(kcpName)
	kubeadm := generateKubeadmConfigTemplate(kubeadmTemplateName)
	kcpYaml, err := yaml.Marshal(kcp)
	if err != nil {
		return nil, err
	}
	kubeadmYaml, err := yaml.Marshal(kubeadm)
	if err != nil {
		return nil, err
	}
	fileStr := string(kcpYaml) + yamlSeparator + string(kubeadmYaml)
	if kubeadmConfigName == "" {
		return []byte(fileStr), nil
	}

	kubeadmConfig := generateKubeadmConfig(kubeadmConfigName)
	kubeadmConfigYaml, err := yaml.Marshal(kubeadmConfig)
	if err != nil {
		return nil, err
	}
	fileStr = fileStr + yamlSeparator + string(kubeadmConfigYaml)

	return []byte(fileStr), nil
}

func generateKubeadmConfigTemplate(name string) *cabpkv1.KubeadmConfigTemplate {
	kubeadmSpec := generateKubeadmConfigSpec()
	return &cabpkv1.KubeadmConfigTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfigTemplate",
			APIVersion: cabpkv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cabpkv1.KubeadmConfigTemplateSpec{
			Template: cabpkv1.KubeadmConfigTemplateResource{
				Spec: *kubeadmSpec,
			},
		},
	}
}

func generateKubeadmConfig(name string) *cabpkv1.KubeadmConfig {
	kubeadmSpec := generateKubeadmConfigSpec()
	return &cabpkv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: kcpv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *kubeadmSpec,
	}
}

func generateKubeadmControlPlane(name string) *kcpv1.KubeadmControlPlane {
	kubeadmSpec := generateKubeadmConfigSpec()
	return &kcpv1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: kcpv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: kcpv1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: *kubeadmSpec,
			Version:           "${KUBERNETES_VERSION}",
		},
	}
}

func generateKubeadmConfigSpec() *cabpkv1.KubeadmConfigSpec {
	return &cabpkv1.KubeadmConfigSpec{
		Files: []cabpkv1.File{
			{
				Path:        "/usr/local/bin/ci-artifacts.sh",
				Content:     debianInjectionScriptBytes,
				Owner:       "root:root",
				Permissions: "0750",
			},
		},
		PreKubeadmCommands: []string{"/usr/local/bin/ci-artifacts.sh"},
	}
}
