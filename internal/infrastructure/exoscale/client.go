package exoscale

import (
	"context"
	"errors"
	"fmt"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/google/uuid"
)

var _ domain.ExoscaleClient = (*client)(nil)

type client struct {
	exoClient egoscale.Client
}

func NewClient(apiKey, apisecret string, zone egoscale.ZoneName) (*client, error) {
	exoClient, err := egoscale.NewClient(credentials.NewStaticCredentials(apiKey, apisecret))
	if err != nil {
		return nil, fmt.Errorf("unable to create exoscale client: %w", err)
	}

	endpoint, err := exoClient.GetZoneAPIEndpoint(context.Background(), zone)
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoint for zone %q: %w", string(zone), err)
	}

	exoClient = exoClient.WithEndpoint(endpoint)

	return &client{exoClient: *exoClient}, nil
}

// CreateElasticIP create a managed elastic IP.
func (c *client) CreateElasticIP(ctx context.Context, healthCheckPort int32, description string) (uuid.UUID, error) {
	op, err := c.exoClient.CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
		Description: description,
		Healthcheck: &egoscale.ElasticIPHealthcheck{
			Mode: egoscale.ElasticIPHealthcheckModeTCP,
			Port: int64(healthCheckPort),
		},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to create elastic ip: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for the elastic IP creation: %w", err)
	}

	id, err := uuid.Parse(op.Reference.ID.String())
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create elastic ip: %w", err)
	}

	return id, nil
}

func (c *client) GetElasticIP(ctx context.Context, id uuid.UUID) (domain.ElasticIP, error) {
	elasticIP, err := c.exoClient.GetElasticIP(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.ElasticIPNotFound
		}
		return domain.ElasticIP{}, err
	}

	var healthcheckPort int32
	if elasticIP.Healthcheck != nil {
		healthcheckPort = int32(elasticIP.Healthcheck.Port)
	}

	return domain.ElasticIP{
		ID:              id,
		IP:              elasticIP.IP,
		Description:     elasticIP.Description,
		HealthCheckPort: healthcheckPort,
	}, nil
}

func (c *client) UpdateElasticIP(ctx context.Context, eip domain.ElasticIP) error {
	op, err := c.exoClient.UpdateElasticIP(ctx, egoscale.UUID(eip.ID.String()), egoscale.UpdateElasticIPRequest{
		Description: eip.Description,
		Healthcheck: &egoscale.ElasticIPHealthcheck{
			Port: int64(eip.HealthCheckPort),
			Mode: egoscale.ElasticIPHealthcheckModeTCP,
		},
	})

	if err != nil {
		return fmt.Errorf("unable to update elastic ip: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return fmt.Errorf("error while waiting for the elastic IP update: %w", err)
	}

	return nil
}

func (c *client) DeleteElasticIP(ctx context.Context, id uuid.UUID) error {
	op, err := c.exoClient.DeleteElasticIP(ctx, egoscale.UUID(id.String()))
	if err != nil {
		return fmt.Errorf("unable to delete elastic ip: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return fmt.Errorf("error while waiting for the elastic IP deletion: %w", err)
	}

	return nil
}

func (c *client) CreateSecurityGroup(ctx context.Context, name string) (uuid.UUID, error) {
	op, err := c.exoClient.CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
		Name: name,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to create security group: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for the security group creation: %w", err)
	}

	id, err := uuid.Parse(op.Reference.ID.String())
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create security group: %w", err)
	}

	return id, nil
}

func (c *client) GetSecurityGroup(ctx context.Context, id uuid.UUID) (domain.SecurityGroup, error) {
	sg, err := c.exoClient.GetSecurityGroup(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.SecurityGroupNotFound
		}
		return domain.SecurityGroup{}, err
	}

	return domain.SecurityGroup{
		ID:   id,
		Name: sg.Name,
	}, nil
}

func (c *client) DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error {
	op, err := c.exoClient.DeleteSecurityGroup(ctx, egoscale.UUID(id.String()))
	if err != nil {
		return fmt.Errorf("unable to delete security group: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return fmt.Errorf("error while waiting for the security group deletion: %w", err)
	}

	return nil
}

func (c *client) CreateSecurityGroupRule(ctx context.Context, sgID uuid.UUID, rule domain.SecurityGroupRule) (uuid.UUID, error) {
	req := egoscale.AddRuleToSecurityGroupRequest{
		Description:   rule.Description,
		FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(rule.FlowDirection),
		Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(rule.Protocol),
		StartPort:     rule.StartPort,
		EndPort:       rule.EndPort,
	}

	if rule.Network != nil {
		req.Network = *rule.Network
	}

	if rule.SecurityGroup != nil {
		req.SecurityGroup = &egoscale.SecurityGroupResource{ID: egoscale.UUID(rule.SecurityGroup.String())}
	}

	op, err := c.exoClient.AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), req)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to create security group rule: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for security group rule creation: %w", err)
	}

	id, err := uuid.Parse(op.Reference.ID.String())
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create security group rule: %w", err)
	}

	return id, nil
}

func (c *client) DeleteSecurityGroupRule(ctx context.Context, sgID uuid.UUID, ruleID uuid.UUID) error {
	op, err := c.exoClient.DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String()))
	if err != nil {
		return fmt.Errorf("unable to delete security group rule: %w", err)
	}

	if _, err := c.exoClient.Wait(ctx, op); err != nil {
		return fmt.Errorf("error while waiting for security group rule deletion: %w", err)
	}

	return nil
}

func (c *client) ListSecurityGroupRules(ctx context.Context, sgID uuid.UUID) ([]domain.SecurityGroupRule, error) {
	sg, err := c.exoClient.GetSecurityGroup(ctx, egoscale.UUID(sgID.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.SecurityGroupNotFound
		}
		return nil, err
	}

	rules := make([]domain.SecurityGroupRule, 0, len(sg.Rules))
	for _, r := range sg.Rules {
		id, err := uuid.Parse(r.ID.String())
		if err != nil {
			return nil, fmt.Errorf("unable to parse security group rule ID: %w", err)
		}

		rule := domain.SecurityGroupRule{
			ID:            id,
			Description:   r.Description,
			FlowDirection: domain.SecurityGroupRuleFlowDirection(r.FlowDirection),
			Protocol:      domain.SecurityGroupRuleProtocol(r.Protocol),
			StartPort:     r.StartPort,
			EndPort:       r.EndPort,
		}

		if r.Network != "" {
			rule.Network = &r.Network
		}

		if r.SecurityGroup != nil {
			sgID, err := uuid.Parse(r.SecurityGroup.ID.String())
			if err != nil {
				return nil, fmt.Errorf("unable to parse security group rule source ID: %w", err)
			}
			rule.SecurityGroup = &sgID
		}

		rules = append(rules, rule)
	}

	return rules, nil
}
