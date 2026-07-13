/*
Copyright 2026.

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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ExoscaleClusterTemplateSpec defines the desired state of ExoscaleClusterTemplate
type ExoscaleClusterTemplateSpec struct {
	Template ExoscaleClusterTemplateResource `json:"template"`
}

// ExoscaleClusterTemplateResource contains spec for ExoscaleClusterSpec.
// cf. https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster#infraclustertemplate-infraclustertemplatelist-resource-definition
type ExoscaleClusterTemplateResource struct {
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	Spec       ExoscaleClusterSpec  `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=exoscaleclustertemplates,scope=Namespaced,categories=cluster-api

// ExoscaleClusterTemplate is the Schema for the exoscaleclustertemplates API
type ExoscaleClusterTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ExoscaleClusterTemplate
	// +required
	Spec ExoscaleClusterTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ExoscaleClusterTemplateList contains a list of ExoscaleClusterTemplate
type ExoscaleClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ExoscaleClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExoscaleClusterTemplate{}, &ExoscaleClusterTemplateList{})
}
