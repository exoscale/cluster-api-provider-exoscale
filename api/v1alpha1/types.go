package v1alpha1

import (
	egoscale "github.com/exoscale/egoscale/v3"
)

// APIEndpoint represents a reachable Kubernetes API endpoint.
// +kubebuilder:validation:MinProperties=1
type APIEndpoint struct {
	// host is the hostname on which the API server is serving (will be setup by the controller).
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Host string `json:"host,omitempty"`

	// port is the port on which the API server is serving.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=6443
	Port int32 `json:"port,omitempty"`
}

type APIEndpointStatus struct {
	APIEndpoint `json:",inline"`

	// +optional
	ID string `json:"id,omitempty,omitzero"`
	// +optional
	Description string `json:"description,omitempty,omitzero"`
}

// ExoscaleSecretRef points to a Kubernetes Secret that holds Exoscale API credentials.
// The Secret must be in the same namespace as the ExoscaleCluster that references it.
// The fields identify the Secret by name and the keys within it that contain the API key and secret.
type ExoscaleSecretRef struct {
	// name is the name of the Kubernetes Secret containing the Exoscale API credentials.
	// +kubebuilder:default=exoscale
	Name string `json:"name"`

	// apiKey is the key inside the Secret whose value is the Exoscale API key.
	// +kubebuilder:default=apikey
	ApiKey string `json:"apiKey"`

	// apiSecret is the key inside the Secret whose value is the Exoscale API secret.
	// +kubebuilder:default=apisecret
	APISecret string `json:"apiSecret"`
}

// SecurityGroup defines the firewall rules applied to the cluster nodes.
// It is translated into an Exoscale Security Group that controls inbound and outbound traffic.
type SecurityGroup struct {
	// rules is the list of firewall rules that make up this security group.
	// +optional
	Rules []SecurityGroupRule `json:"rules,omitempty"`
}

// SecurityGroupRules defines a single firewall rule within a SecurityGroup.
// A rule matches traffic by protocol, port range, and source/destination network,
// and applies to either ingress or egress direction.
type SecurityGroupRule struct {
	// flowDirection defines whether this rule applies to incoming (ingress) or outgoing (egress) traffic.
	// +required
	// +kubebuilder:validation:Enum=ingress;egress
	// +kubebuilder:default=ingress
	FlowDirection egoscale.SecurityGroupRuleFlowDirection `json:"flowDirection"`

	// protocol is the network protocol this rule applies to.
	// +required
	// +kubebuilder:validation:Enum=tcp;udp;icmp;icmpv6;esp;ah;gre;ipip
	// +kubebuilder:default=tcp
	Protocol egoscale.SecurityGroupRuleProtocol `json:"protocol"`

	// startPort is the first port of the port range this rule applies to (inclusive).
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	StartPort int64 `json:"startPort,omitempty"`

	// endPort is the last port of the port range this rule applies to (inclusive).
	// Must be greater than or equal to startPort.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	EndPort int64 `json:"endPort,omitempty"`

	// network is the source (ingress) or destination (egress) network in CIDR notation (e.g. "0.0.0.0/0").
	// +optional
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}\/([0-9]|[1-2][0-9]|3[0-2])$|^([0-9a-fA-F:]+)\/([0-9]|[1-9][0-9]|1[0-1][0-9]|12[0-8])$`
	Network *string `json:"network,omitempty"`

	// securityGroup identifies a security group as the source (ingress) or destination (egress) for this rule,
	// as an alternative to a CIDR network. Accepts either the UUID of an existing Exoscale security group,
	// or one of the special values "control-plane" / "worker" to refer to this cluster's managed
	// control-plane or node security group.
	// +optional
	SecurityGroup *string `json:"securityGroup,omitempty"`

	// description is a human-readable explanation of what this rule does.
	// +required
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`
}

// Security Group
type SecurityGroupResource struct {
	// Security Group ID
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	ID string `json:"id,omitempty"`
}

type SecurityGroupStatus struct {
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	ID string `json:"id,omitempty"`
	// +optional
	Name string `json:"name,omitempty,omitzero"`

	// rules is the list of firewall rules that make up this security group.
	// +optional
	Rules []SecurityGroupRuleStatus `json:"rules,omitempty,omitzero"`
}

type SecurityGroupRuleStatus struct {
	SecurityGroupRule `json:",inline"`
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	ID string `json:"id,omitempty"`

	SecurityGroupRef *SecurityGroupResource `json:"securityGroupRef,omitempty"`
}

// ExoscaleClusterInitializationStatus provides observations of the ExoscaleCluster initialization process.
// +kubebuilder:validation:MinProperties=1
type ExoscaleClusterInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Cluster provisioning.
	// see: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster#infracluster-initialization-completed
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}
