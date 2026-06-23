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
	egoscale "github.com/exoscale/egoscale/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ReadyCondition reports the overall readiness of the ExoscaleCluster.
	ReadyCondition = "Ready"

	// ReconcileSuccessReason surfaces when the last reconciliation completed without errors.
	ReconcileSuccessReason = "ReconcileSuccess"

	// ReconcileErrorReason surfaces when the last reconciliation returned an error.
	ReconcileErrorReason = "ReconcileError"

	ExoscaleClusterFinalizer string = "exoscalecluster.infrastructure.cluster.x-k8s.io/finalizer"
)

// ExoscaleClusterSpec defines the desired state of ExoscaleCluster
// this resources should meet the required specs described by the clusterAPI: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster
type ExoscaleClusterSpec struct {

	// controlPlaneEndpoint is the host and port through which the Kubernetes API server is reachable.
	// You do not need to set this manually — the controller fills it in once the control plane is provisioned.
	// See: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster#infracluster-control-plane-endpoint
	// +optional
	// +kubebuilder:default={port: 6443}
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty,omitzero"`

	// exoscaleSecret references a Kubernetes Secret holding the Exoscale API key and secret
	// that the controller uses to provision and manage cloud resources for this cluster.
	// The referenced Secret must exist in the same namespace as this resource.
	// +optional
	// +kubebuilder:default={name: "exoscale", apiKey: "apikey", apiSecret: "apisecret"}
	ExoscaleSecret ExoscaleSecretRef `json:"exoscaleSecret"`

	// zone is the Exoscale datacenter where the cluster will be provisioned.
	// See https://www.exoscale.com/datacenters/ for the list of available datacenters.
	// +required
	// +kubebuilder:validation:Enum=at-vie-1;at-vie-2;bg-sof-1;ch-dk-2;ch-gva-2;de-fra-1;de-muc-1;hr-zag-1
	Zone egoscale.ZoneName `json:"zone"`

	// SecurityGroupControlPlane defines additional firewall rules applied to the cluster control plane.
	// The controller automatically creates the base rules required for Kubernetes node
	// and the control plane to communicate. Use this field to add extra rules on top,
	// for example to allow ssh traffic on the control plane resource.
	// +optional
	SecurityGroupControlPlane SecurityGroup `json:"securityGroupControlPlane,omitempty"`

	// SecurityGroupNode defines additional firewall rules applied to the cluster nodes.
	// The controller automatically creates the basic rules required to operate a Kubernetes cluster.
	// Use this field to add extra rules on top, for example to allow traffic for your CNI plugin.
	// +optional
	SecurityGroupNode SecurityGroup `json:"securityGroupNode,omitempty"`
}

// ExoscaleClusterStatus defines the observed state of ExoscaleCluster.
type ExoscaleClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the ExoscaleCluster resource.
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

	// initialization provides observations of the HetznerCluster initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
	// see: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster#infracluster-initialization-completed
	// +optional
	Initialization ExoscaleClusterInitializationStatus `json:"initialization,omitempty,omitzero"`

	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	ID *string `json:"id,omitempty"`

	// +optional
	SecurityGroupControlPlan *SecurityGroupStatus `json:"securityGroupControlPlane,omitempty,omitzero"`
	// +optional
	SecurityGroupNode *SecurityGroupStatus `json:"securityGroupNode,omitempty,omitzero"`

	// +optional
	ControlPlaneEndpoint *APIEndpointStatus `json:"controlPlaneEndpoint,omitempty,omitzero"`
}

// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".spec.zone",description="Zone used to deploy the cluster"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.controlPlaneEndpoint.host",description="API Endpoint"
// +kubebuilder:printcolumn:name="Port",type="string",JSONPath=".status.controlPlaneEndpoint.port",description="API Endpoint"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description="Cluster infrastructure is ready for Nodes"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of HetznerCluster"
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=exoscaleclusters,scope=Namespaced,categories=cluster-api
// ExoscaleCluster is the Schema for the exoscaleclusters API
type ExoscaleCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ExoscaleCluster
	// +required
	Spec ExoscaleClusterSpec `json:"spec"`

	// status defines the observed state of ExoscaleCluster
	// +optional
	Status ExoscaleClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ExoscaleClusterList contains a list of ExoscaleCluster
type ExoscaleClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ExoscaleCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExoscaleCluster{}, &ExoscaleClusterList{})
}

// GetConditions returns the conditions for ExoscaleCluster, implementing the conditions.Getter interface.
func (e *ExoscaleCluster) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

// SetConditions sets the conditions for ExoscaleCluster, implementing the conditions.Setter interface.
func (e *ExoscaleCluster) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
}
