/*
Copyright 2019 The Kubernetes Authors.

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

// ExoscaleClusterProviderSpecSpec defines the desired state of ExoscaleClusterProviderSpec
type ExoscaleClusterProviderSpecSpec struct {
	Zone          string `json:"zone"`
	SecurityGroup string `json:"securityGroup"`
}

// ExoscaleClusterProviderSpecStatus defines the observed state of ExoscaleClusterProviderSpec
type ExoscaleClusterProviderSpecStatus struct {
	Zone          string `json:"zone"`
	SecurityGroup string `json:"securityGroup"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleClusterProviderSpec is the Schema for the exoscaleclusterproviderconfigs API
// +k8s:openapi-gen=true
type ExoscaleClusterProviderSpec struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExoscaleClusterProviderSpecSpec   `json:"spec,omitempty"`
	Status ExoscaleClusterProviderSpecStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleClusterProviderSpecList contains a list of ExoscaleClusterProviderSpec
type ExoscaleClusterProviderSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ExoscaleClusterProviderSpec `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExoscaleClusterProviderSpec{}, &ExoscaleClusterProviderSpecList{})
}
