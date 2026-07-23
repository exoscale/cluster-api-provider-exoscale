package service

import (
	"context"
	"testing"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_securityGroupService_UpsertSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clusterID := uuid.New()
	sgID := uuid.New()
	name := "test-security-group"

	sg := domain.SecurityGroup{
		ID:   sgID,
		Name: name,
	}

	tests := []struct {
		name   string
		scID   *uuid.UUID
		cloud  func(m *mocks.Cloud)
		output domain.SecurityGroup
		err    error
	}{
		{
			name: "nominal - no scID",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListSecurityGroups(ctx).Return(nil, nil)
				m.EXPECT().
					CreateSecurityGroup(ctx, name).
					Return(sgID, nil)
				m.EXPECT().
					GetSecurityGroup(ctx, sgID).
					Return(sg, nil)
			},
			output: sg,
		},
		{
			name: "recovers existing security group by name",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListSecurityGroups(ctx).Return([]domain.SecurityGroup{sg}, nil)
			},
			output: sg,
		},
		{
			name: "stale status ID recovers existing security group by name",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						GetSecurityGroup(ctx, sgID).
						Return(domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound).
						Once(),
					m.EXPECT().
						ListSecurityGroups(ctx).
						Return([]domain.SecurityGroup{sg}, nil).
						Once(),
				)
			},
			output: sg,
		},
		{
			name: "rejects status security group owned by another cluster",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(domain.SecurityGroup{ID: sgID, Name: "foreign"}, nil)
			},
			err: errNotOwned,
		},
		{
			name: "nominal - scID found",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetSecurityGroup(ctx, sgID).
					Return(sg, nil)
			},
			output: sg,
		},
		{
			name: "get security group returned an error",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetSecurityGroup(ctx, sgID).
					Return(domain.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "create security group returned an error - no scID",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListSecurityGroups(ctx).Return(nil, nil)
				m.EXPECT().
					CreateSecurityGroup(ctx, name).
					Return(uuid.Nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "create security group returned an error - scID not found",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						GetSecurityGroup(ctx, sgID).
						Return(domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound).
						Once(),
					m.EXPECT().
						ListSecurityGroups(ctx).
						Return(nil, nil).
						Once(),
					m.EXPECT().
						CreateSecurityGroup(ctx, name).
						Return(uuid.Nil, assert.AnError).
						Once(),
				)
			},
			err: assert.AnError,
		},
		{
			name: "get new security group returned an error - no scID",
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						ListSecurityGroups(ctx).
						Return(nil, nil).
						Once(),
					m.EXPECT().
						CreateSecurityGroup(ctx, name).
						Return(sgID, nil).
						Once(),
					m.EXPECT().
						GetSecurityGroup(ctx, sgID).
						Return(domain.SecurityGroup{}, assert.AnError).
						Once(),
				)
			},
			err: assert.AnError,
		},
		{
			name: "get new security group returned an error - scID not found",
			scID: &sgID,
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						GetSecurityGroup(ctx, sgID).
						Return(domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound).
						Once(),
					m.EXPECT().
						ListSecurityGroups(ctx).
						Return(nil, nil).
						Once(),
					m.EXPECT().
						CreateSecurityGroup(ctx, name).
						Return(sgID, nil).
						Once(),
					m.EXPECT().
						GetSecurityGroup(ctx, sgID).
						Return(domain.SecurityGroup{}, assert.AnError).
						Once(),
				)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			cloud := mocks.NewCloud(t)
			if ut.cloud != nil {
				ut.cloud(cloud)
			}

			svc := securityGroupService{cloud: cloud, logger: logr.Discard()}

			output, err := svc.UpsertSecurityGroup(ctx, clusterID, ut.scID, name)

			if ut.err == errNotOwned {
				assert.ErrorContains(t, err, "is not owned by cluster")
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
			assert.Equal(t, ut.output, output)
		})
	}

	t.Run("rejects duplicate security groups by name", func(t *testing.T) {
		cloud := mocks.NewCloud(t)
		cloud.EXPECT().ListSecurityGroups(ctx).Return([]domain.SecurityGroup{sg, {ID: uuid.New(), Name: name}}, nil)
		svc := securityGroupService{cloud: cloud, logger: logr.Discard()}

		_, err := svc.UpsertSecurityGroup(ctx, clusterID, nil, name)

		assert.ErrorContains(t, err, "multiple security groups")
	})
}

func Test_securityGroupService_DeleteSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	name := "expected"

	tests := []struct {
		name  string
		cloud func(m *mocks.Cloud)
		err   error
	}{
		{
			name: "nominal - sg found",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, id).Return(domain.SecurityGroup{ID: id, Name: name}, nil)
				m.EXPECT().DeleteSecurityGroup(ctx, id).Return(nil)
			},
		},
		{
			name: "rejects foreign security group",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, id).Return(domain.SecurityGroup{ID: id, Name: "foreign"}, nil)
			},
			err: errNotOwned,
		},
		{
			name: "nominal - sg not found",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, id).Return(domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound)
			},
		},
		{
			name: "get security group returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, id).Return(domain.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "delete security group returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, id).Return(domain.SecurityGroup{ID: id, Name: name}, nil)
				m.EXPECT().DeleteSecurityGroup(ctx, id).Return(assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			cloud := mocks.NewCloud(t)
			if ut.cloud != nil {
				ut.cloud(cloud)
			}

			svc := securityGroupService{cloud: cloud, logger: logr.Discard()}

			err := svc.DeleteSecurityGroup(ctx, id, name)

			if ut.err == errNotOwned {
				assert.ErrorContains(t, err, "expected")
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
		})
	}
}

func Test_securityGroupService_PurgeSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	sgName := "expected"
	sg := domain.SecurityGroup{ID: sgID, Name: sgName}
	rule1ID := uuid.New()
	rule2ID := uuid.New()

	rules := []domain.SecurityGroupRule{
		{ID: rule1ID, Description: "rule-1"},
		{ID: rule2ID, Description: "rule-2"},
	}

	tests := []struct {
		name  string
		cloud func(m *mocks.Cloud)
		err   error
	}{
		{
			name: "nominal - no rules",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(sg, nil)
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(nil, nil)
			},
		},
		{
			name: "nominal - security group already deleted",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetSecurityGroup(ctx, sgID).
					Return(domain.SecurityGroup{}, domain.ErrSecurityGroupNotFound)
			},
		},
		{
			name: "nominal - rules deleted",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(sg, nil)
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(rules, nil)
				m.EXPECT().
					DeleteSecurityGroupRule(ctx, sgID, rule1ID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroupRule(ctx, sgID, rule2ID).
					Return(nil)
			},
		},
		{
			name: "rejects foreign security group before deleting rules",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(domain.SecurityGroup{ID: sgID, Name: "foreign"}, nil)
			},
			err: errNotOwned,
		},
		{
			name: "list rules returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(sg, nil)
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "delete rule returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetSecurityGroup(ctx, sgID).Return(sg, nil)
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(rules, nil)
				m.EXPECT().
					DeleteSecurityGroupRule(ctx, sgID, rule1ID).
					Return(assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			cloud := mocks.NewCloud(t)
			if ut.cloud != nil {
				ut.cloud(cloud)
			}

			svc := securityGroupService{cloud: cloud, logger: logr.Discard()}

			err := svc.PurgeSecurityGroup(ctx, sgID, sgName)

			if ut.err == errNotOwned {
				assert.ErrorContains(t, err, "expected")
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
		})
	}
}

func Test_securityGroupService_UpsertSecurityGroupRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()

	// keptRule exists in cloud and is also in the desired state — stays untouched.
	keptRule := domain.SecurityGroupRule{
		ID:            uuid.New(),
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
	}
	// ruleToDelete exists in cloud but is absent from the desired state — must be removed.
	ruleToDelete := domain.SecurityGroupRule{
		ID:            uuid.New(),
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolUDP,
		StartPort:     53,
		EndPort:       53,
	}
	// desiredKeptRule mirrors keptRule's policy without an ID (desired state has no cloud IDs).
	desiredKeptRule := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
	}
	// ruleToCreate is in the desired state but absent from cloud — must be created.
	ruleToCreate := domain.SecurityGroupRule{
		FlowDirection: domain.SecurityGroupRuleFlowDirectionEgress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     443,
		EndPort:       443,
	}
	// createdRule is ruleToCreate after the cloud assigns it an ID.
	createdRule := domain.SecurityGroupRule{
		ID:            uuid.New(),
		FlowDirection: domain.SecurityGroupRuleFlowDirectionEgress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     443,
		EndPort:       443,
	}

	tests := []struct {
		name         string
		desiredRules []domain.SecurityGroupRule
		cloud        func(m *mocks.Cloud)
		output       []domain.SecurityGroupRule
		err          error
	}{
		{
			// existing: [keptRule, ruleToDelete]
			// desired:  [desiredKeptRule, ruleToCreate]
			name:         "nominal - add, delete and keep rules",
			desiredRules: []domain.SecurityGroupRule{desiredKeptRule, ruleToCreate},
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return([]domain.SecurityGroupRule{keptRule, ruleToDelete}, nil)
				m.EXPECT().
					DeleteSecurityGroupRule(ctx, sgID, ruleToDelete.ID).
					Return(nil)
				m.EXPECT().
					CreateSecurityGroupRule(ctx, sgID, ruleToCreate).
					Return(createdRule.ID, nil)
			},
			output: []domain.SecurityGroupRule{keptRule, createdRule},
		},
		{
			name: "list rules returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name:         "delete rule returned an error",
			desiredRules: nil,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return([]domain.SecurityGroupRule{keptRule}, nil)
				m.EXPECT().
					DeleteSecurityGroupRule(ctx, sgID, keptRule.ID).
					Return(assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name:         "create rule returned an error",
			desiredRules: []domain.SecurityGroupRule{ruleToCreate},
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListSecurityGroupRules(ctx, sgID).
					Return(nil, nil)
				m.EXPECT().
					CreateSecurityGroupRule(ctx, sgID, ruleToCreate).
					Return(uuid.Nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			cloud := mocks.NewCloud(t)
			if ut.cloud != nil {
				ut.cloud(cloud)
			}

			svc := securityGroupService{cloud: cloud, logger: logr.Discard()}

			output, err := svc.UpsertSecurityGroupRules(ctx, sgID, ut.desiredRules)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}
