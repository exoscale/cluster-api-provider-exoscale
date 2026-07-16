package exoscale

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"net"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/google/uuid"
)

var _ domain.Cloud = (*cloud)(nil)

const operationWaitTimeout = 10 * time.Minute

type cloud struct {
	exoClient      domain.ExoscaleClient
	instanceClient instanceClient
}

type instanceClient interface {
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

func NewCloud(apiKey, apisecret string, zone egoscale.ZoneName) (*cloud, error) {
	exoClient, err := egoscale.NewClient(
		credentials.NewStaticCredentials(apiKey, apisecret),
		egoscale.ClientOptWithWaitTimeout(operationWaitTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create exoscale client: %w", err)
	}

	endpoint, err := exoClient.GetZoneAPIEndpoint(context.Background(), zone)
	if err != nil {
		return nil, fmt.Errorf("unable to get endpoint for zone %q: %w", string(zone), err)
	}

	exoClient = exoClient.WithEndpoint(endpoint)

	return &cloud{exoClient: exoClient, instanceClient: exoClient}, nil
}

func (c *cloud) waitForSuccess(ctx context.Context, op *egoscale.Operation) (*egoscale.Operation, error) {
	completed, err := c.exoClient.Wait(ctx, op, egoscale.OperationStateSuccess)
	if err != nil {
		return nil, err
	}
	if completed == nil || completed.State != egoscale.OperationStateSuccess {
		return nil, errors.New("operation did not succeed")
	}
	return completed, nil
}

func operationReferenceID(op *egoscale.Operation) (uuid.UUID, error) {
	if op == nil || op.Reference == nil {
		return uuid.Nil, errors.New("operation returned no reference")
	}

	parsed, err := uuid.Parse(op.Reference.ID.String())
	if err != nil {
		return uuid.Nil, err
	}
	if parsed == uuid.Nil {
		return uuid.Nil, errors.New("operation reference ID is nil")
	}
	return parsed, nil
}

// CreateElasticIP creates a managed elastic IP and waits for the operation to complete.
func (c *cloud) CreateElasticIP(ctx context.Context, healthCheckPort int32, description string) (uuid.UUID, error) {
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

	completed, err := c.waitForSuccess(ctx, op)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for the elastic IP creation: %w", err)
	}

	id, err := operationReferenceID(completed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create elastic ip: %w", err)
	}

	return id, nil
}

func (c *cloud) GetElasticIP(ctx context.Context, id uuid.UUID) (domain.ElasticIP, error) {
	elasticIP, err := c.exoClient.GetElasticIP(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.ErrElasticIPNotFound
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

func (c *cloud) ListElasticIPs(ctx context.Context) ([]domain.ElasticIP, error) {
	resp, err := c.exoClient.ListElasticIPS(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to list elastic IPs: %w", err)
	}

	eips := make([]domain.ElasticIP, 0, len(resp.ElasticIPS))
	for _, e := range resp.ElasticIPS {
		id, err := uuid.Parse(e.ID.String())
		if err != nil {
			return nil, fmt.Errorf("unable to parse elastic IP ID: %w", err)
		}

		var healthcheckPort int32
		if e.Healthcheck != nil {
			healthcheckPort = int32(e.Healthcheck.Port)
		}

		eips = append(eips, domain.ElasticIP{
			ID:              id,
			IP:              e.IP,
			Description:     e.Description,
			HealthCheckPort: healthcheckPort,
		})
	}

	return eips, nil
}

func (c *cloud) UpdateElasticIP(ctx context.Context, eip domain.ElasticIP) error {
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

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for the elastic IP update: %w", err)
	}

	return nil
}

func (c *cloud) DeleteElasticIP(ctx context.Context, id uuid.UUID) error {
	op, err := c.exoClient.DeleteElasticIP(ctx, egoscale.UUID(id.String()))
	if err != nil {
		return fmt.Errorf("unable to delete elastic ip: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for the elastic IP deletion: %w", err)
	}

	return nil
}

func (c *cloud) CreateSecurityGroup(ctx context.Context, name string) (uuid.UUID, error) {
	op, err := c.exoClient.CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
		Name: name,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to create security group: %w", err)
	}

	completed, err := c.waitForSuccess(ctx, op)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for the security group creation: %w", err)
	}

	id, err := operationReferenceID(completed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create security group: %w", err)
	}

	return id, nil
}

func (c *cloud) GetSecurityGroup(ctx context.Context, id uuid.UUID) (domain.SecurityGroup, error) {
	sg, err := c.exoClient.GetSecurityGroup(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.ErrSecurityGroupNotFound
		}
		return domain.SecurityGroup{}, err
	}

	return domain.SecurityGroup{
		ID:   id,
		Name: sg.Name,
	}, nil
}

func (c *cloud) DeleteSecurityGroup(ctx context.Context, id uuid.UUID) error {
	op, err := c.exoClient.DeleteSecurityGroup(ctx, egoscale.UUID(id.String()))
	if err != nil {
		return fmt.Errorf("unable to delete security group: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for the security group deletion: %w", err)
	}

	return nil
}

func (c *cloud) CreateSecurityGroupRule(ctx context.Context, sgID uuid.UUID, rule domain.SecurityGroupRule) (uuid.UUID, error) {
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

	completed, err := c.waitForSuccess(ctx, op)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for security group rule creation: %w", err)
	}

	id, err := operationReferenceID(completed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create security group rule: %w", err)
	}

	return id, nil
}

func (c *cloud) DeleteSecurityGroupRule(ctx context.Context, sgID uuid.UUID, ruleID uuid.UUID) error {
	op, err := c.exoClient.DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String()))
	if err != nil {
		return fmt.Errorf("unable to delete security group rule: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for security group rule deletion: %w", err)
	}

	return nil
}

func (c *cloud) ListSecurityGroupRules(ctx context.Context, sgID uuid.UUID) ([]domain.SecurityGroupRule, error) {
	sg, err := c.exoClient.GetSecurityGroup(ctx, egoscale.UUID(sgID.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.ErrSecurityGroupNotFound
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

func (c *cloud) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	client, err := c.instances()
	if err != nil {
		return nil, err
	}

	resp, err := client.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to list instances: %w", err)
	}

	instances := make([]domain.Instance, 0, len(resp.Instances))
	for _, instance := range resp.Instances {
		id, err := uuid.Parse(instance.ID.String())
		if err != nil {
			return nil, fmt.Errorf("unable to parse instance ID: %w", err)
		}
		securityGroupIDs, err := instanceSecurityGroupIDs(instance.SecurityGroups)
		if err != nil {
			return nil, err
		}

		instances = append(instances, domain.Instance{
			ID:               id,
			Name:             instance.Name,
			State:            string(instance.State),
			PublicIP:         ipString(instance.PublicIP),
			CreatedAt:        formatTime(instance.CreatedAT),
			Labels:           copyLabels(instance.Labels),
			SecurityGroupIDs: securityGroupIDs,
		})
	}

	return instances, nil
}

func (c *cloud) ListInstanceTypes(ctx context.Context) ([]domain.InstanceType, error) {
	client, err := c.instances()
	if err != nil {
		return nil, err
	}

	instanceTypes, err := client.ListInstanceTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to list instance types: %w", err)
	}

	out := make([]domain.InstanceType, 0, len(instanceTypes.InstanceTypes))
	for _, instanceType := range instanceTypes.InstanceTypes {
		out = append(out, domain.InstanceType{
			ID:     instanceType.ID.String(),
			Family: string(instanceType.Family),
			Size:   string(instanceType.Size),
		})
	}

	return out, nil
}

func (c *cloud) GetTemplate(ctx context.Context, id uuid.UUID) (domain.InstanceTemplate, error) {
	client, err := c.instances()
	if err != nil {
		return domain.InstanceTemplate{}, err
	}

	template, err := client.GetTemplate(ctx, egoscale.UUID(id.String()))
	if err != nil {
		return domain.InstanceTemplate{}, err
	}

	return mapTemplate(*template)
}

func (c *cloud) ListTemplates(ctx context.Context) ([]domain.InstanceTemplate, error) {
	client, err := c.instances()
	if err != nil {
		return nil, err
	}

	templates, err := client.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]domain.InstanceTemplate, 0, len(templates.Templates))
	for _, template := range templates.Templates {
		mapped, err := mapTemplate(template)
		if err != nil {
			return nil, err
		}
		out = append(out, mapped)
	}

	return out, nil
}

func (c *cloud) CreateInstance(ctx context.Context, spec domain.ResolvedInstanceSpec) (uuid.UUID, error) {
	client, err := c.instances()
	if err != nil {
		return uuid.Nil, err
	}

	req := egoscale.CreateInstanceRequest{
		DiskSize: spec.DiskSizeGB,
		InstanceType: &egoscale.InstanceType{
			ID:     egoscale.UUID(spec.InstanceType.ID),
			Family: egoscale.InstanceTypeFamily(spec.InstanceType.Family),
			Size:   egoscale.InstanceTypeSize(spec.InstanceType.Size),
		},
		Labels:             egoscale.Labels(spec.Labels),
		Name:               spec.Name,
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     securityGroups(spec.SecurityGroupIDs),
		Template:           &egoscale.Template{ID: egoscale.UUID(spec.TemplateID.String())},
	}
	if spec.SSHKey != "" {
		req.SSHKey = &egoscale.SSHKey{Name: spec.SSHKey}
	}
	if spec.UserData != "" {
		req.UserData = base64.StdEncoding.EncodeToString([]byte(spec.UserData))
	}

	op, err := client.CreateInstance(ctx, req)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to create instance: %w", err)
	}

	completed, err := c.waitForSuccess(ctx, op)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error while waiting for instance creation: %w", err)
	}

	id, err := operationReferenceID(completed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to parse response from create instance: %w", err)
	}

	return id, nil
}

func (c *cloud) GetInstance(ctx context.Context, id uuid.UUID) (domain.Instance, error) {
	client, err := c.instances()
	if err != nil {
		return domain.Instance{}, err
	}

	instance, err := client.GetInstance(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			err = domain.ErrInstanceNotFound
		}
		return domain.Instance{}, err
	}

	securityGroupIDs, err := instanceSecurityGroupIDs(instance.SecurityGroups)
	if err != nil {
		return domain.Instance{}, err
	}

	return domain.Instance{
		ID:               id,
		Name:             instance.Name,
		State:            string(instance.State),
		PublicIP:         ipString(instance.PublicIP),
		CreatedAt:        formatTime(instance.CreatedAT),
		Labels:           copyLabels(instance.Labels),
		SecurityGroupIDs: securityGroupIDs,
	}, nil
}

func (c *cloud) AttachInstanceToElasticIP(ctx context.Context, instanceID, elasticIPID uuid.UUID) error {
	client, err := c.instances()
	if err != nil {
		return err
	}

	op, err := client.AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), egoscale.AttachInstanceToElasticIPRequest{
		Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
	})
	if err != nil {
		return fmt.Errorf("unable to attach instance to elastic IP: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for instance attachment to elastic IP: %w", err)
	}

	return nil
}

func (c *cloud) AttachInstanceToSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error {
	client, err := c.instances()
	if err != nil {
		return err
	}

	op, err := client.AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.AttachInstanceToSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	})
	if err != nil {
		return fmt.Errorf("unable to attach instance to security group: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for instance attachment to security group: %w", err)
	}

	return nil
}

func (c *cloud) DetachInstanceFromSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error {
	client, err := c.instances()
	if err != nil {
		return err
	}

	op, err := client.DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.DetachInstanceFromSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	})
	if err != nil {
		return fmt.Errorf("unable to detach instance from security group: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for instance detachment from security group: %w", err)
	}

	return nil
}

func (c *cloud) DeleteInstance(ctx context.Context, id uuid.UUID) error {
	client, err := c.instances()
	if err != nil {
		return err
	}

	op, err := client.DeleteInstance(ctx, egoscale.UUID(id.String()))
	if err != nil {
		if errors.Is(err, egoscale.ErrNotFound) {
			return domain.ErrInstanceNotFound
		}
		return fmt.Errorf("unable to delete instance: %w", err)
	}

	_, err = c.waitForSuccess(ctx, op)
	if err != nil {
		return fmt.Errorf("error while waiting for instance deletion: %w", err)
	}

	return nil
}

func (c *cloud) instances() (instanceClient, error) {
	if c.instanceClient == nil {
		return nil, fmt.Errorf("exoscale client does not support instances")
	}
	return c.instanceClient, nil
}

func securityGroups(ids []uuid.UUID) []egoscale.SecurityGroup {
	groups := make([]egoscale.SecurityGroup, 0, len(ids))
	for _, id := range ids {
		groups = append(groups, egoscale.SecurityGroup{ID: egoscale.UUID(id.String())})
	}
	return groups
}

func instanceSecurityGroupIDs(groups []egoscale.SecurityGroup) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(groups))
	for _, group := range groups {
		id, err := uuid.Parse(group.ID.String())
		if err != nil {
			return nil, fmt.Errorf("unable to parse instance security group ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func mapTemplate(template egoscale.Template) (domain.InstanceTemplate, error) {
	id, err := uuid.Parse(template.ID.String())
	if err != nil {
		return domain.InstanceTemplate{}, fmt.Errorf("unable to parse template id %q: %w", template.ID, err)
	}

	return domain.InstanceTemplate{ID: id, Name: template.Name, SizeBytes: template.Size, CreatedAt: template.CreatedAT}, nil
}

func ipString(ip net.IP) string {
	if len(ip) == 0 {
		return ""
	}
	return ip.String()
}

func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	maps.Copy(out, labels)
	return out
}
