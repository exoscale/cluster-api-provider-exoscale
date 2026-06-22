package exoscale

import (
	"context"
	"testing"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/stretchr/testify/assert"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

func Test_cloud_CreateElasticIP(t *testing.T) {
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
				).Return(&egoscale.Operation{}, nil)
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
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
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

			client := cloud{exoClient: exoClient}

			output, err := client.CreateElasticIP(ctx, int32(healthCheckPort), description)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_GetElasticIP(t *testing.T) {
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

			client := cloud{exoClient: exoClient}

			output, err := client.GetElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_UpdateElasticIP(t *testing.T) {
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
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

			client := cloud{exoClient: exoClient}

			err := client.UpdateElasticIP(ctx, eip)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_DeleteElasticIP(t *testing.T) {
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
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

			client := cloud{exoClient: exoClient}

			err := client.DeleteElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_CreateSecurityGroup(t *testing.T) {
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
				).Return(&egoscale.Operation{}, nil)
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
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
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

			client := cloud{exoClient: exoClient}

			output, err := client.CreateSecurityGroup(ctx, name)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_GetSecurityGroup(t *testing.T) {
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

			client := cloud{exoClient: exoClient}

			output, err := client.GetSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_DeleteSecurityGroup(t *testing.T) {
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
					Wait(ctx, &egoscale.Operation{}).
					Return(&egoscale.Operation{}, nil)
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
					Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
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

			client := cloud{exoClient: exoClient}

			err := client.DeleteSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_CreateSecurityGroupRule(t *testing.T) {
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
				).Return(&egoscale.Operation{}, nil)
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
				).Return(&egoscale.Operation{}, nil)
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

			client := cloud{exoClient: exoClient}

			output, err := client.CreateSecurityGroupRule(ctx, sgID, ut.rule)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_DeleteSecurityGroupRule(t *testing.T) {
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
					Wait(ctx, &egoscale.Operation{}).
					Return(&egoscale.Operation{}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
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

			client := cloud{exoClient: exoClient}

			err := client.DeleteSecurityGroupRule(ctx, sgID, ruleID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_ListSecurityGroupRules(t *testing.T) {
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

			client := cloud{exoClient: exoClient}

			output, err := client.ListSecurityGroupRules(ctx, sgID)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}
