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

// ExoscaleMachineSpec defines the desired state of an Exoscale compute instance
// backing a Cluster API Machine.
// Zone is inherited from ExoscaleCluster.spec.zone. If multi-zone support is
// added later, use CAPI Machine.spec.failureDomain to pick the target zone
// instead of duplicating zone here.
type ExoscaleMachineSpec struct {
	// template is an Exoscale instance template UUID or exact template name.
	// UUIDs pin an exact template; names are resolved at create time.
	// +required
	// +kubebuilder:validation:MinLength=1
	Template string `json:"template"`

	// instanceType is an Exoscale instance type UUID or a value in [family.]size
	// format. For example, "small" is shorthand for "standard.small".
	// The value is resolved against the Exoscale catalog during reconciliation.
	// Run `exo compute instance-type list -v` to list available values.
	// +required
	// +kubebuilder:validation:MinLength=1
	InstanceType string `json:"instanceType"`

	// sshKey is the name of a pre-existing SSH key registered in the Exoscale project.
	// +optional
	SSHKey string `json:"sshKey,omitempty"`

	// securityGroups lists UUIDs of Exoscale Security Groups to attach in addition
	// to the cluster's control-plane or node security group.
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// rootVolumeSizeGiB overrides the disk size declared by the template.
	// +optional
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=10000
	RootVolumeSizeGiB *int64 `json:"rootVolumeSizeGiB,omitempty"`

	// providerID is the cloud-provider identifier for this instance in the form
	// exoscale://<instance-uuid>. Set by the controller after the instance is created.
	// CAPI uses this field to match the InfraMachine to the Node object.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`
}

// ExoscaleMachineInitializationStatus provides observations of the ExoscaleMachine initialization process.
// +kubebuilder:validation:MinProperties=1
type ExoscaleMachineInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate Machine provisioning.
	// see: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine#inframachine-initialization-completed
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// ExoscaleMachineStatus defines the observed state of an ExoscaleMachine.
// Fields follow the CAPI InfraMachine contract:
// https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine
type ExoscaleMachineStatus struct {
	// initialization provides observations of the Machine infrastructure initialization process.
	// +optional
	Initialization ExoscaleMachineInitializationStatus `json:"initialization,omitempty,omitzero"`

	// ready is true when the Exoscale instance is running and reachable.
	Ready bool `json:"ready"`

	// instanceID is the Exoscale instance UUID assigned after creation.
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	InstanceID string `json:"instanceID,omitempty"`

	// addresses are the network addresses of the instance (public and private IPs).
	// +optional
	// +listType=atomic
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// failureReason is a short machine-readable token set on terminal errors.
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// failureMessage is a human-readable elaboration of failureReason.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// conditions represents the observations of the current state of the ExoscaleMachine.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=exoscalemachines,scope=Namespaced,categories=cluster-api,shortName=exom
// +kubebuilder:storageversion
// +kubebuilder:validation:XValidation:rule="self.spec.template == oldSelf.spec.template && self.spec.instanceType == oldSelf.spec.instanceType && has(self.spec.sshKey) == has(oldSelf.spec.sshKey) && (!has(self.spec.sshKey) || self.spec.sshKey == oldSelf.spec.sshKey) && has(self.spec.securityGroups) == has(oldSelf.spec.securityGroups) && (!has(self.spec.securityGroups) || self.spec.securityGroups == oldSelf.spec.securityGroups) && has(self.spec.rootVolumeSizeGiB) == has(oldSelf.spec.rootVolumeSizeGiB) && (!has(self.spec.rootVolumeSizeGiB) || self.spec.rootVolumeSizeGiB == oldSelf.spec.rootVolumeSizeGiB)",message="instance creation fields are immutable"
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.metadata.labels.cluster\.x-k8s\.io/cluster-name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.spec.providerID`
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

func (e *ExoscaleMachine) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *ExoscaleMachine) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
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
