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
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExoscaleMachineTemplateSpec defines the desired state of ExoscaleMachineTemplate
type ExoscaleMachineTemplateSpec struct {
	// template contains the metadata and spec copied into generated ExoscaleMachines.
	// +required
	Template ExoscaleMachineTemplateResource `json:"template"`
}

// ExoscaleMachineTemplateResource contains the metadata and spec copied into an ExoscaleMachine.
type ExoscaleMachineTemplateResource struct {
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// +required
	Spec ExoscaleMachineSpec `json:"spec"`
}

// ExoscaleMachineTemplateStatus defines the observed state of ExoscaleMachineTemplate.
type ExoscaleMachineTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the ExoscaleMachineTemplate resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ExoscaleMachineTemplate is the Schema for the exoscalemachinetemplates API
type ExoscaleMachineTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ExoscaleMachineTemplate
	// +required
	Spec ExoscaleMachineTemplateSpec `json:"spec"`

	// status defines the observed state of ExoscaleMachineTemplate
	// +optional
	Status ExoscaleMachineTemplateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ExoscaleMachineTemplateList contains a list of ExoscaleMachineTemplate
type ExoscaleMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ExoscaleMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ExoscaleMachineTemplate{}, &ExoscaleMachineTemplateList{})
		return nil
	})
}
