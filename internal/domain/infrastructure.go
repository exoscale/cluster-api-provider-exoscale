package domain

import (
	"context"

	"github.com/google/uuid"
)

type ExoscaleClient interface {
	CreateElasticIP(ctx context.Context, healthCheckPort int32, description string) (uuid.UUID, error)
	GetElasticIP(ctx context.Context, id uuid.UUID) (ElasticIP, error)
	UpdateElasticIP(ctx context.Context, eip ElasticIP) error
	DeleteElasticIP(ctx context.Context, id uuid.UUID) error

	CreateSecurityGroup(ctx context.Context, name string) (uuid.UUID, error)
	GetSecurityGroup(ctx context.Context, id uuid.UUID) (SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error

	CreateSecurityGroupRule(ctx context.Context, sgID uuid.UUID, rule SecurityGroupRule) (uuid.UUID, error)
	DeleteSecurityGroupRule(ctx context.Context, sgID uuid.UUID, ruleID uuid.UUID) error
	ListSecurityGroupRules(ctx context.Context, sgID uuid.UUID) ([]SecurityGroupRule, error)

	CreateInstance(ctx context.Context, name string, spec InstanceSpec) (uuid.UUID, error)
	GetInstance(ctx context.Context, id uuid.UUID) (Instance, error)
	DeleteInstance(ctx context.Context, id uuid.UUID) error
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

type Cluster struct {
	ID       *uuid.UUID
	Endpoint ElasticIPService
}

type InstanceService interface {
	UpsertInstance(ctx context.Context, machineID uuid.UUID, instanceID *uuid.UUID, spec InstanceSpec) (Instance, error)
	DeleteInstance(ctx context.Context, id uuid.UUID) error
}

type InstanceSpec struct {
	Zone             string
	TemplateID       uuid.UUID
	InstanceType     string
	SSHKey           string
	SecurityGroupIDs []uuid.UUID
	RootVolumeSizeGB *int64
}

type Instance struct {
	ID        uuid.UUID
	Name      string
	State     string
	PublicIP  string
	PrivateIP string
	CreatedAt string
}
