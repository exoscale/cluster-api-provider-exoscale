/*
Copyright 2018 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExoscaleClusterProviderStatusSpec defines the desired state of ExoscaleClusterProviderStatus
type ExoscaleClusterProviderStatusSpec struct {
	Zone          string `json:"zone"`
	SSHKey        string `json:"sshkey"`
	SecurityGroup string `json:"security-group"`
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// ExoscaleClusterProviderStatusStatus defines the observed state of ExoscaleClusterProviderStatus
type ExoscaleClusterProviderStatusStatus struct {
	Zone          string `json:"zone"`
	SSHKey        string `json:"sshkey"`
	SecurityGroup string `json:"security-group"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleClusterProviderStatus is the Schema for the exoscaleclusterproviderstatuses API
// +k8s:openapi-gen=true
type ExoscaleClusterProviderStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExoscaleClusterProviderStatusSpec   `json:"spec,omitempty"`
	Status ExoscaleClusterProviderStatusStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleClusterProviderStatusList contains a list of ExoscaleClusterProviderStatus
type ExoscaleClusterProviderStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExoscaleClusterProviderStatus `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExoscaleClusterProviderStatus{}, &ExoscaleClusterProviderStatusList{})
}
