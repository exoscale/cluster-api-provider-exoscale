package domain

import (
	"context"
	"time"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

const (
	// ClusterIDKey identifies the annotation and cloud label used for cluster ownership.
	ClusterIDKey = "cluster-api-provider-exoscale/cluster-id"
	// MachineUIDKey identifies the annotation and cloud label used for Machine ownership.
	MachineUIDKey = "cluster-api-provider-exoscale/machine-uid"
)

// ExoscaleClient is the subset of the Exoscale SDK used by the infrastructure adapter.
type ExoscaleClient interface {
	Wait(ctx context.Context, op *egoscale.Operation, states ...egoscale.OperationState) (*egoscale.Operation, error)

	CreateElasticIP(ctx context.Context, req egoscale.CreateElasticIPRequest) (*egoscale.Operation, error)
	GetElasticIP(ctx context.Context, id egoscale.UUID) (*egoscale.ElasticIP, error)
	ListElasticIPS(ctx context.Context) (*egoscale.ListElasticIPSResponse, error)
	UpdateElasticIP(ctx context.Context, id egoscale.UUID, req egoscale.UpdateElasticIPRequest) (*egoscale.Operation, error)
	DeleteElasticIP(ctx context.Context, id egoscale.UUID) (*egoscale.Operation, error)

	CreateSecurityGroup(ctx context.Context, req egoscale.CreateSecurityGroupRequest) (*egoscale.Operation, error)
	GetSecurityGroup(ctx context.Context, id egoscale.UUID) (*egoscale.SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, id egoscale.UUID) (*egoscale.Operation, error)
	AddRuleToSecurityGroup(ctx context.Context, id egoscale.UUID, req egoscale.AddRuleToSecurityGroupRequest) (*egoscale.Operation, error)
	DeleteRuleFromSecurityGroup(ctx context.Context, id egoscale.UUID, ruleID egoscale.UUID) (*egoscale.Operation, error)

	ListInstances(ctx context.Context, opts ...egoscale.ListInstancesOpt) (*egoscale.ListInstancesResponse, error)
	CreateInstance(ctx context.Context, req egoscale.CreateInstanceRequest) (*egoscale.Operation, error)
	GetInstance(ctx context.Context, id egoscale.UUID) (*egoscale.Instance, error)
	AttachInstanceToElasticIP(ctx context.Context, id egoscale.UUID, req egoscale.AttachInstanceToElasticIPRequest) (*egoscale.Operation, error)
	AttachInstanceToSecurityGroup(ctx context.Context, id egoscale.UUID, req egoscale.AttachInstanceToSecurityGroupRequest) (*egoscale.Operation, error)
	DetachInstanceFromSecurityGroup(ctx context.Context, id egoscale.UUID, req egoscale.DetachInstanceFromSecurityGroupRequest) (*egoscale.Operation, error)
	DeleteInstance(ctx context.Context, id egoscale.UUID) (*egoscale.Operation, error)
	ListInstanceTypes(ctx context.Context) (*egoscale.ListInstanceTypesResponse, error)
	GetTemplate(ctx context.Context, id egoscale.UUID) (*egoscale.Template, error)
	ListTemplates(ctx context.Context, opts ...egoscale.ListTemplatesOpt) (*egoscale.ListTemplatesResponse, error)
}

// Cloud exposes the cluster-scoped cloud operations used by the cluster services.
type Cloud interface {
	CreateElasticIP(ctx context.Context, healthCheckPort int32, description string) (uuid.UUID, error)
	GetElasticIP(ctx context.Context, id uuid.UUID) (ElasticIP, error)
	ListElasticIPs(ctx context.Context) ([]ElasticIP, error)
	UpdateElasticIP(ctx context.Context, eip ElasticIP) error
	DeleteElasticIP(ctx context.Context, id uuid.UUID) error

	CreateSecurityGroup(ctx context.Context, name string) (uuid.UUID, error)
	GetSecurityGroup(ctx context.Context, id uuid.UUID) (SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error
	CreateSecurityGroupRule(ctx context.Context, sgID uuid.UUID, rule SecurityGroupRule) (uuid.UUID, error)
	DeleteSecurityGroupRule(ctx context.Context, sgID, ruleID uuid.UUID) error
	ListSecurityGroupRules(ctx context.Context, sgID uuid.UUID) ([]SecurityGroupRule, error)

	ListInstances(ctx context.Context, label string) ([]Instance, error)
	ListInstanceTypes(ctx context.Context) ([]InstanceType, error)
	GetTemplate(ctx context.Context, id uuid.UUID) (InstanceTemplate, error)
	ListTemplates(ctx context.Context) ([]InstanceTemplate, error)
	CreateInstance(ctx context.Context, spec ResolvedInstanceSpec) (uuid.UUID, error)
	GetInstance(ctx context.Context, id uuid.UUID) (Instance, error)
	AttachInstanceToElasticIP(ctx context.Context, instanceID, elasticIPID uuid.UUID) error
	AttachInstanceToSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error
	DetachInstanceFromSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error
	DeleteInstance(ctx context.Context, id uuid.UUID) error
}

// ElasticIPService reconciles the Elastic IP owned by a cluster.
type ElasticIPService interface {
	UpsertElasticIP(ctx context.Context, clusterID uuid.UUID, eipID *uuid.UUID, port int32) (ElasticIP, error)
	DeleteElasticIP(ctx context.Context, id uuid.UUID) error
}

// SecurityGroupService reconciles cluster Security Groups and their rules.
type SecurityGroupService interface {
	UpsertSecurityGroup(ctx context.Context, clusterID uuid.UUID, scID *uuid.UUID, name string) (SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error
	UpsertSecurityGroupRules(ctx context.Context, sgID uuid.UUID, desiredRules []SecurityGroupRule) ([]SecurityGroupRule, error)
	PurgeSecurityGroup(ctx context.Context, sgID uuid.UUID) error
}

// ElasticIP is the provider-independent representation of an Exoscale Elastic IP.
type ElasticIP struct {
	ID              uuid.UUID
	IP              string
	Description     string
	HealthCheckPort int32
}

// SecurityGroup is the provider-independent representation of an Exoscale Security Group.
type SecurityGroup struct {
	ID   uuid.UUID
	Name string
}

// SecurityGroupRuleFlowDirection identifies whether a rule applies to ingress or egress traffic.
type SecurityGroupRuleFlowDirection string

const (
	// SecurityGroupRuleFlowDirectionIngress matches incoming traffic.
	SecurityGroupRuleFlowDirectionIngress SecurityGroupRuleFlowDirection = "ingress"
	// SecurityGroupRuleFlowDirectionEgress matches outgoing traffic.
	SecurityGroupRuleFlowDirectionEgress SecurityGroupRuleFlowDirection = "egress"
)

// SecurityGroupRuleProtocol identifies the network protocol matched by a Security Group rule.
type SecurityGroupRuleProtocol string

const (
	// SecurityGroupRuleProtocolTCP matches TCP traffic.
	SecurityGroupRuleProtocolTCP SecurityGroupRuleProtocol = "tcp"
	// SecurityGroupRuleProtocolEsp matches ESP traffic.
	SecurityGroupRuleProtocolEsp SecurityGroupRuleProtocol = "esp"
	// SecurityGroupRuleProtocolICMP matches ICMP traffic.
	SecurityGroupRuleProtocolICMP SecurityGroupRuleProtocol = "icmp"
	// SecurityGroupRuleProtocolUDP matches UDP traffic.
	SecurityGroupRuleProtocolUDP SecurityGroupRuleProtocol = "udp"
	// SecurityGroupRuleProtocolGre matches GRE traffic.
	SecurityGroupRuleProtocolGre SecurityGroupRuleProtocol = "gre"
	// SecurityGroupRuleProtocolAh matches AH traffic.
	SecurityGroupRuleProtocolAh SecurityGroupRuleProtocol = "ah"
	// SecurityGroupRuleProtocolIpip matches IP-in-IP traffic.
	SecurityGroupRuleProtocolIpip SecurityGroupRuleProtocol = "ipip"
	// SecurityGroupRuleProtocolIcmpv6 matches ICMPv6 traffic.
	SecurityGroupRuleProtocolIcmpv6 SecurityGroupRuleProtocol = "icmpv6"
)

// SecurityGroupRule describes one provider-independent firewall rule.
type SecurityGroupRule struct {
	ID            uuid.UUID
	Description   string
	FlowDirection SecurityGroupRuleFlowDirection
	Protocol      SecurityGroupRuleProtocol
	StartPort     int64
	EndPort       int64
	Network       *string
	SecurityGroup *uuid.UUID
}

// ClusterService reconciles the cloud resources shared by a CAPI Cluster.
type ClusterService interface {
	ReconcileCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error)
	DeleteCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error)
}

// InstanceService reconciles the Exoscale instance owned by a CAPI Machine.
type InstanceService interface {
	UpsertInstance(ctx context.Context, machineUID MachineUID, instanceID *uuid.UUID, spec InstanceSpec) (Instance, error)
	DeleteInstance(ctx context.Context, machineUID MachineUID, clusterID uuid.UUID, instanceID *uuid.UUID) error
}

// MachineUID is the stable ownership value derived from a Kubernetes Machine UID.
type MachineUID string

// String returns the Machine UID as a string.
func (uid MachineUID) String() string { return string(uid) }

// InstanceSpec contains the user-facing values required to resolve an instance request.
type InstanceSpec struct {
	Name              string
	Template          string
	InstanceType      string
	SSHKey            string
	SecurityGroupIDs  []uuid.UUID
	ElasticIPID       *uuid.UUID
	RootVolumeSizeGiB *int64
	UserData          string
	Labels            map[string]string
}

// ResolvedInstanceSpec contains the concrete provider values required to create an instance.
type ResolvedInstanceSpec struct {
	Name             string
	TemplateID       uuid.UUID
	InstanceType     InstanceType
	SSHKey           string
	SecurityGroupIDs []uuid.UUID
	DiskSizeGiB      int64
	UserData         string
	Labels           map[string]string
}

// InstanceTemplate describes an Exoscale instance template available for provisioning.
type InstanceTemplate struct {
	ID        uuid.UUID
	Name      string
	SizeBytes int64
	CreatedAt time.Time
}

// InstanceType describes an Exoscale compute offering.
type InstanceType struct {
	ID     string
	Family string
	Size   string
}

// Instance is the provider-independent representation of an Exoscale instance.
type Instance struct {
	ID               uuid.UUID
	Name             string
	State            string
	PublicIP         string
	PrivateIP        string
	CreatedAt        string
	Labels           map[string]string
	SecurityGroupIDs []uuid.UUID
}
