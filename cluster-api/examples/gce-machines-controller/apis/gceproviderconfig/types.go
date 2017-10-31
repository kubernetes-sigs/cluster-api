package gceproviderconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GCEProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	Project     string `json:"project"`
	Zone        string `json:"zone"`
	MachineType string `json:"machineType"`
	Image       string `json:"image"`
}
