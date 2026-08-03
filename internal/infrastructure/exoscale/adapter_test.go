package exoscale

import (
	"context"
	"encoding/base64"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

func Test_adapter_CreateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	healthCheckPort := 12345
	description := "description"
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{
					State:     egoscale.OperationStateSuccess,
					Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())},
				}, nil)
			},
			output: id,
		},
		{
			name: "create eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: "123456"}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.CreateElasticIP(ctx, int32(healthCheckPort), description)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_GetElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.ElasticIP
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(
						&egoscale.ElasticIP{
							Description: "description",
							IP:          "1.2.3.4",
							Healthcheck: &egoscale.ElasticIPHealthcheck{
								Port: 12345,
							},
						}, nil)
			},
			output: domain.ElasticIP{
				ID:              id,
				IP:              "1.2.3.4",
				HealthCheckPort: 12345,
				Description:     "description",
			},
		},
		{
			name: "get eip returned an error - not fond",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.ElasticIP{}, egoscale.ErrNotFound)
			},
			err: domain.ErrElasticIPNotFound,
		},
		{
			name: "get eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.ElasticIP{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.GetElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_ListElasticIPs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id1 := uuid.New()
	id2 := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.ElasticIP
		err       error
	}{
		{
			name: "nominal - empty list",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).
					Return(&egoscale.ListElasticIPSResponse{ElasticIPS: []egoscale.ElasticIP{}}, nil)
			},
			output: []domain.ElasticIP{},
		},
		{
			name: "nominal - maps fields correctly",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).
					Return(&egoscale.ListElasticIPSResponse{
						ElasticIPS: []egoscale.ElasticIP{
							{
								ID:          egoscale.UUID(id1.String()),
								IP:          "1.2.3.4",
								Description: "capi - clusterID - abc",
								Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 6443},
							},
							{
								ID:          egoscale.UUID(id2.String()),
								IP:          "5.6.7.8",
								Description: "other",
								Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 12345},
							},
						},
					}, nil)
			},
			output: []domain.ElasticIP{
				{ID: id1, IP: "1.2.3.4", Description: "capi - clusterID - abc", HealthCheckPort: 6443},
				{ID: id2, IP: "5.6.7.8", Description: "other", HealthCheckPort: 12345},
			},
		},
		{
			name: "list eips returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.ListElasticIPs(ctx)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_CreateInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	securityGroupID := uuid.New()
	diskSizeGiB := int64(20)
	instanceTypeID := egoscale.UUID(uuid.New().String())
	instanceType := egoscale.InstanceType{
		ID:     instanceTypeID,
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	spec := domain.ResolvedInstanceSpec{
		Name:       "machine-0",
		TemplateID: templateID,
		InstanceType: domain.InstanceType{
			ID:     instanceType.ID.String(),
			Family: string(instanceType.Family),
			Size:   string(instanceType.Size),
		},
		SSHKey:           "ssh-key",
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
		DiskSizeGiB:      diskSizeGiB,
		UserData:         "#cloud-config",
		Labels:           map[string]string{"machine": "uid"},
	}

	createOp := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
	createOpSuccess := &egoscale.Operation{
		Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())},
		State:     egoscale.OperationStateSuccess,
	}
	exoClient := mocks.NewExoscaleClient(t)
	exoClient.EXPECT().CreateInstance(ctx, egoscale.CreateInstanceRequest{
		DiskSize:           diskSizeGiB,
		InstanceType:       &instanceType,
		Labels:             egoscale.Labels{"machine": "uid"},
		Name:               "machine-0",
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
		SSHKey:             &egoscale.SSHKey{Name: "ssh-key"},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
		UserData:           base64.StdEncoding.EncodeToString([]byte("#cloud-config")),
	}).Return(createOp, nil)

	exoClient.EXPECT().Wait(ctx, createOp, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(createOpSuccess, nil)

	client := Adapter{client: exoClient}

	output, err := client.CreateInstance(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, instanceID, output)
}

func Test_adapter_UpdateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	eip := domain.ElasticIP{
		ID:              id,
		IP:              "1.2.3.4",
		Description:     "description",
		HealthCheckPort: 12345,
	}

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "update eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.UpdateElasticIP(ctx, eip)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_DeleteElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "delete eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.DeleteElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_CreateSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	name := "sg-name"

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{
					Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())},
					State:     egoscale.OperationStateSuccess}, nil)
			},
			output: id,
		},
		{
			name: "create security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: "123456"}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: "123456"}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.CreateSecurityGroup(ctx, name)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_GetSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.SecurityGroup
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{Name: "sg-name"}, nil)
			},
			output: domain.SecurityGroup{
				ID:   id,
				Name: "sg-name",
			},
		},
		{
			name: "get security group returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{}, egoscale.ErrNotFound)
			},
			err: domain.ErrSecurityGroupNotFound,
		},
		{
			name: "get security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.GetSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_DeleteSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "delete security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.DeleteSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_CreateSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()
	network := "10.0.0.0/8"

	baseRule := domain.SecurityGroupRule{
		Description:   "allow tcp",
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
	}

	tests := []struct {
		name      string
		rule      domain.SecurityGroupRule
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal - with network",
			rule: domain.SecurityGroupRule{
				Description:   baseRule.Description,
				FlowDirection: baseRule.FlowDirection,
				Protocol:      baseRule.Protocol,
				StartPort:     baseRule.StartPort,
				EndPort:       baseRule.EndPort,
				Network:       &network,
			},
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					Network:       network,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{
					State:     egoscale.OperationStateSuccess,
					Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())},
				}, nil)
			},
			output: ruleID,
		},
		{
			name: "nominal - with source security group",
			rule: domain.SecurityGroupRule{
				Description:   baseRule.Description,
				FlowDirection: baseRule.FlowDirection,
				Protocol:      baseRule.Protocol,
				StartPort:     baseRule.StartPort,
				EndPort:       baseRule.EndPort,
				SecurityGroup: &sgID,
			},
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sgID.String())},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{
					State:     egoscale.OperationStateSuccess,
					Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())},
				}, nil)
			},
			output: ruleID,
		},
		{
			name: "add rule returned an error",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.CreateSecurityGroupRule(ctx, sgID, ut.rule)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_DeleteSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "delete rule returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.DeleteSecurityGroupRule(ctx, sgID, ruleID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_ListSecurityGroupRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID1 := uuid.New()
	ruleID2 := uuid.New()
	sourceSGID := uuid.New()
	network := "10.0.0.0/8"

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.SecurityGroupRule
		err       error
	}{
		{
			name: "nominal - empty rules",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{Rules: []egoscale.SecurityGroupRule{}}, nil)
			},
			output: []domain.SecurityGroupRule{},
		},
		{
			name: "nominal - rule with network",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{
						Rules: []egoscale.SecurityGroupRule{
							// With network as the source
							{
								ID:            egoscale.UUID(ruleID1.String()),
								Description:   "allow tcp",
								FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
								Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
								StartPort:     80,
								EndPort:       80,
								Network:       network,
							},
							// With security group as the source
							{
								ID:            egoscale.UUID(ruleID2.String()),
								Description:   "allow from sg",
								FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
								Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
								StartPort:     443,
								EndPort:       443,
								SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sourceSGID.String())},
							},
						},
					}, nil)
			},
			output: []domain.SecurityGroupRule{
				{
					ID:            ruleID1,
					Description:   "allow tcp",
					FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      domain.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Network:       &network,
				},
				{
					ID:            ruleID2,
					Description:   "allow from sg",
					FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      domain.SecurityGroupRuleProtocolTCP,
					StartPort:     443,
					EndPort:       443,
					SecurityGroup: &sourceSGID,
				},
			},
		},
		{
			name: "get security group returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{}, egoscale.ErrNotFound)
			},
			err: domain.ErrSecurityGroupNotFound,
		},
		{
			name: "get security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.ListSecurityGroupRules(ctx, sgID)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_ListInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	label := "cluster-api-provider-exoscale/cluster-id=abc"
	id1 := uuid.New()
	sgID := uuid.New()
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.Instance
		err       error
	}{
		{
			name: "nominal - maps fields correctly",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListInstances(ctx, mock.MatchedBy(func(opts []egoscale.ListInstancesOpt) bool {
					values := url.Values{}
					for _, opt := range opts {
						opt(values)
					}
					return values.Get("labels") == label
				})).
					Return(&egoscale.ListInstancesResponse{
						Instances: []egoscale.ListInstancesResponseInstances{
							{
								ID:             egoscale.UUID(id1.String()),
								Name:           "machine-0",
								State:          egoscale.InstanceStateRunning,
								PublicIP:       net.ParseIP("1.2.3.4"),
								CreatedAT:      createdAt,
								Labels:         egoscale.Labels{"machine": "uid"},
								SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(sgID.String())}},
							},
						},
					}, nil)
			},
			output: []domain.Instance{
				{
					ID:               id1,
					Name:             "machine-0",
					State:            string(egoscale.InstanceStateRunning),
					PublicIP:         "1.2.3.4",
					CreatedAt:        createdAt.Format(time.RFC3339),
					Labels:           map[string]string{"machine": "uid"},
					SecurityGroupIDs: []uuid.UUID{sgID},
				},
			},
		},
		{
			name: "list instances returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListInstances(ctx, mock.Anything).
					Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.ListInstances(ctx, label)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_ListInstanceTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := egoscale.UUID(uuid.New().String())

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.InstanceType
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListInstanceTypes(ctx).Return(&egoscale.ListInstanceTypesResponse{
					InstanceTypes: []egoscale.InstanceType{
						{ID: id, Family: egoscale.InstanceTypeFamilyStandard, Size: egoscale.InstanceTypeSize("small")},
					},
				}, nil)
			},
			output: []domain.InstanceType{
				{ID: id.String(), Family: string(egoscale.InstanceTypeFamilyStandard), Size: "small"},
			},
		},
		{
			name: "list instance types returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListInstanceTypes(ctx).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.ListInstanceTypes(ctx)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_GetTemplate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.InstanceTemplate
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetTemplate(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Template{
						ID:        egoscale.UUID(id.String()),
						Name:      "linux-24.04",
						Size:      1073741824,
						CreatedAT: createdAt,
					}, nil)
			},
			output: domain.InstanceTemplate{
				ID:        id,
				Name:      "linux-24.04",
				SizeBytes: 1073741824,
				CreatedAt: createdAt,
			},
		},
		{
			name: "get template returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetTemplate(ctx, egoscale.UUID(id.String())).
					Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.GetTemplate(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_ListTemplates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.InstanceTemplate
		err       error
	}{
		{
			name: "nominal - empty list",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListTemplates(ctx).Return(&egoscale.ListTemplatesResponse{}, nil)
			},
			output: []domain.InstanceTemplate{},
		},
		{
			name: "nominal - maps fields correctly",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListTemplates(ctx).Return(&egoscale.ListTemplatesResponse{
					Templates: []egoscale.Template{
						{ID: egoscale.UUID(id.String()), Name: "linux-24.04", Size: 1073741824, CreatedAT: createdAt},
					},
				}, nil)
			},
			output: []domain.InstanceTemplate{
				{ID: id, Name: "linux-24.04", SizeBytes: 1073741824, CreatedAt: createdAt},
			},
		},
		{
			name: "list templates returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListTemplates(ctx).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.ListTemplates(ctx)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_GetInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sgID := uuid.New()
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.Instance
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Instance{
						Name:           "machine-0",
						State:          egoscale.InstanceStateRunning,
						PublicIP:       net.ParseIP("1.2.3.4"),
						CreatedAT:      createdAt,
						Labels:         egoscale.Labels{"machine": "uid"},
						SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(sgID.String())}},
					}, nil)
			},
			output: domain.Instance{
				ID:               id,
				Name:             "machine-0",
				State:            string(egoscale.InstanceStateRunning),
				PublicIP:         "1.2.3.4",
				CreatedAt:        createdAt.Format(time.RFC3339),
				Labels:           map[string]string{"machine": "uid"},
				SecurityGroupIDs: []uuid.UUID{sgID},
			},
		},
		{
			name: "get instance returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Instance{}, egoscale.ErrNotFound)
			},
			err: domain.ErrInstanceNotFound,
		},
		{
			name: "get instance returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Instance{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			output, err := client.GetInstance(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_adapter_AttachInstanceToElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	elasticIPID := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), egoscale.AttachInstanceToElasticIPRequest{
					Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "attach returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), egoscale.AttachInstanceToElasticIPRequest{
					Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), egoscale.AttachInstanceToElasticIPRequest{
					Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.AttachInstanceToElasticIP(ctx, instanceID, elasticIPID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_AttachInstanceToSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.AttachInstanceToSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "attach returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.AttachInstanceToSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.AttachInstanceToSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_DetachInstanceFromSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.DetachInstanceFromSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "detach returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.DetachInstanceFromSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.DetachInstanceFromSecurityGroupRequest{
					Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.DetachInstanceFromSecurityGroup(ctx, instanceID, securityGroupID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_adapter_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
		},
		{
			name: "delete instance returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, egoscale.ErrNotFound)
			},
			err: domain.ErrInstanceNotFound,
		},
		{
			name: "delete instance returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := Adapter{client: exoClient}

			err := client.DeleteInstance(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_ipString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ip     net.IP
		output string
	}{
		{
			name:   "nil IP",
			ip:     nil,
			output: "",
		},
		{
			name:   "empty IP",
			ip:     net.IP{},
			output: "",
		},
		{
			name:   "IPv4",
			ip:     net.ParseIP("1.2.3.4"),
			output: "1.2.3.4",
		},
		{
			name:   "IPv6",
			ip:     net.ParseIP("2001:db8::1"),
			output: "2001:db8::1",
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			t.Parallel()

			output := ipString(ut.ip)

			assert.Equal(t, ut.output, output)
		})
	}
}
