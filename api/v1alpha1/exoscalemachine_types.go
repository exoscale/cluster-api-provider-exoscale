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
)

// ExoscaleMachineSpec defines the desired state of an Exoscale compute instance
// backing a Cluster API Machine.
type ExoscaleMachineSpec struct {
	// zone is the Exoscale datacenter where the instance will be created.
	// +required
	// +kubebuilder:validation:Enum=at-vie-1;at-vie-2;bg-sof-1;ch-dk-2;ch-gva-2;de-fra-1;de-muc-1;hr-zag-1
	Zone string `json:"zone"`

	// templateID is the UUID of an existing Exoscale instance template. Templates
	// bundle image, disk size and snapshot recipe; pick one out-of-band.
	// +required
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	TemplateID string `json:"templateID"`

	// instanceType is the Exoscale service offering (e.g. "small", "medium", "large",
	// "gpu-plus", "dev").
	// +required
	// +kubebuilder:validation:Enum=small;medium;large;extra-large;huge;gpu-plus;gpu-2plus;dev;startup-2;startup-4;startup-8;startup-16;standard;standard-2;standard-4;standard-8;standard-12;standard-16;standard-20;standard-24;standard-32;memory;memory-2;memory-4;memory-8;memory-12;memory-16;memory-20;memory-24;compute;compute-2;compute-4;compute-8;compute-12;compute-16;compute-20;compute-24
	InstanceType string `json:"instanceType"`

	// sshKey is the name of a pre-existing SSH key registered in the Exoscale
	// project. The key is injected into the instance at boot.
	// +required
	SSHKey string `json:"sshKey"`

	// securityGroups are the names or UUIDs of Exoscale Security Groups to attach
	// to the instance. Reference the SGs the ExoscaleCluster controller creates,
	// or any user-provided SG.
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// rootVolumeSizeGB optionally overrides the disk size declared by the
	// template. The instance is recreated if this changes.
	// +optional
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=10000
	RootVolumeSizeGB *int64 `json:"rootVolumeSizeGB,omitempty"`
}

// ExoscaleMachineStatus defines the observed state of an ExoscaleMachine.
// Fields follow the CAPI InfraMachine contract:
// https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine
type ExoscaleMachineStatus struct {
	// conditions represents the observations of the current state of the ExoscaleMachine.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready is true when the instance is running and reachable.
	// +optional
	Ready bool `json:"ready"`

	// instanceID is the Exoscale instance UUID.
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	InstanceID string `json:"instanceID,omitempty"`

	// instanceState is the Exoscale instance state name (e.g. "running", "stopped").
	// +optional
	InstanceState string `json:"instanceState,omitempty"`

	// providerID is the cloud-provider identifier for this instance, in the form
	// exoscale:///<instance-id>. CAPI uses this to associate the Machine with
	// its Node.
	// +optional
	ProviderID string `json:"providerID,omitempty"`

	// addresses are the network addresses of the instance (public and private IPs).
	// +optional
	// +listType=atomic
	Addresses []MachineAddress `json:"addresses,omitempty"`

	// failureReason will be set on a terminal error that prevents reconciliation.
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// failureMessage is a human-readable description of failureReason.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`
}

// MachineAddress models a single network address (matching clusterv1.MachineAddress).
type MachineAddress struct {
	// type is the kind of address (e.g. "InternalIP", "ExternalIP", "Hostname").
	Type string `json:"type"`

	// address is the value of the address.
	Address string `json:"address"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=exoscalemachines,scope=Namespaced,categories=cluster-api,shortName=exom
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.metadata.labels.cluster\.x-k8s\.io/cluster-name`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.instanceState`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="InstanceID",type=string,JSONPath=`.status.instanceID`
// +kubebuilder:printcolumn:name="Machine",type=string,JSONPath=`.metadata.ownerReferences[?(@.kind=="Machine")].name`

// ExoscaleMachine is the Schema for the exoscalemachines API.
type ExoscaleMachine struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ExoscaleMachineSpec `json:"spec"`

	// +optional
	Status ExoscaleMachineStatus `json:"status,omitzero"`
}

// GetConditions returns the conditions on the ExoscaleMachine.
func (in *ExoscaleMachine) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions replaces the conditions on the ExoscaleMachine.
func (in *ExoscaleMachine) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ExoscaleMachineList contains a list of ExoscaleMachine.
type ExoscaleMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ExoscaleMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExoscaleMachine{}, &ExoscaleMachineList{})
}
