package domain

import (
	"context"
	"time"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

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
}

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
	DeleteSecurityGroupRule(ctx context.Context, sgID uuid.UUID, ruleID uuid.UUID) error
	ListSecurityGroupRules(ctx context.Context, sgID uuid.UUID) ([]SecurityGroupRule, error)
}

type ElasticIPService interface {
	UpsertElasticIP(ctx context.Context, clusterID uuid.UUID, eipID *uuid.UUID, port int32) (ElasticIP, error)
	DeleteElasticIP(ctx context.Context, id uuid.UUID) error
}

type SecurityGroupService interface {
	UpsertSecurityGroup(ctx context.Context, clusterID uuid.UUID, scID *uuid.UUID, name string) (SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error
	UpsertSecurityGroupRules(ctx context.Context, sgID uuid.UUID, desiredRules []SecurityGroupRule) ([]SecurityGroupRule, error)
	PurgeSecurityGroup(ctx context.Context, sgID uuid.UUID) error
}

type ElasticIP struct {
	ID              uuid.UUID
	IP              string
	Description     string
	HealthCheckPort int32
}

type SecurityGroup struct {
	ID   uuid.UUID
	Name string
}

type SecurityGroupRuleFlowDirection string

const (
	SecurityGroupRuleFlowDirectionIngress SecurityGroupRuleFlowDirection = "ingress"
	SecurityGroupRuleFlowDirectionEgress  SecurityGroupRuleFlowDirection = "egress"
)

type SecurityGroupRuleProtocol string

const (
	SecurityGroupRuleProtocolTCP    SecurityGroupRuleProtocol = "tcp"
	SecurityGroupRuleProtocolEsp    SecurityGroupRuleProtocol = "esp"
	SecurityGroupRuleProtocolICMP   SecurityGroupRuleProtocol = "icmp"
	SecurityGroupRuleProtocolUDP    SecurityGroupRuleProtocol = "udp"
	SecurityGroupRuleProtocolGre    SecurityGroupRuleProtocol = "gre"
	SecurityGroupRuleProtocolAh     SecurityGroupRuleProtocol = "ah"
	SecurityGroupRuleProtocolIpip   SecurityGroupRuleProtocol = "ipip"
	SecurityGroupRuleProtocolIcmpv6 SecurityGroupRuleProtocol = "icmpv6"
)

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

type ClusterService interface {
	ReconcileCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error)
	DeleteCluster(ctx context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error)
}

type Cluster struct {
	ID       *uuid.UUID
	Endpoint ElasticIPService
}

type InstanceService interface {
	UpsertInstance(ctx context.Context, machineID uuid.UUID, instanceID *uuid.UUID, spec InstanceSpec) (Instance, error)
	DeleteInstance(ctx context.Context, machineID, instanceID *uuid.UUID) error
}

type InstanceSpec struct {
	Name             string
	Template         string
	InstanceType     string
	SSHKey           string
	SecurityGroupIDs []uuid.UUID
	ElasticIPID      *uuid.UUID
	RootVolumeSizeGB *int64
	UserData         string
	Labels           map[string]string
}

type ResolvedInstanceSpec struct {
	Name             string
	TemplateID       uuid.UUID
	InstanceType     InstanceType
	SSHKey           string
	SecurityGroupIDs []uuid.UUID
	DiskSizeGB       int64
	UserData         string
	Labels           map[string]string
}

type InstanceTemplate struct {
	ID        uuid.UUID
	Name      string
	SizeBytes int64
	CreatedAt time.Time
}

type InstanceType struct {
	ID     string
	Family string
	Size   string
}

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
