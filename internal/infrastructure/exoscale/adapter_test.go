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
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAdapter_CreateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
		Description: "description",
		Healthcheck: &egoscale.ElasticIPHealthcheck{
			Mode: egoscale.ElasticIPHealthcheckModeTCP,
			Port: 12345,
		},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{
			State:     egoscale.OperationStateSuccess,
			Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())},
		}, nil)

	got, err := NewAdapter(sdk).CreateElasticIP(ctx, 12345, "description")

	assert.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestAdapter_CreateElasticIP_returnsCreateError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
		Description: "description",
		Healthcheck: &egoscale.ElasticIPHealthcheck{
			Mode: egoscale.ElasticIPHealthcheckModeTCP,
			Port: 12345,
		},
	}).Return(nil, assert.AnError)

	got, err := NewAdapter(sdk).CreateElasticIP(ctx, 12345, "description")

	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, uuid.Nil, got)
}

func TestAdapter_GetElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).Return(&egoscale.ElasticIP{
		Description: "description",
		IP:          "1.2.3.4",
		Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 12345},
	}, nil)

	got, err := NewAdapter(sdk).GetElasticIP(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, domain.ElasticIP{
		ID:              id,
		IP:              "1.2.3.4",
		Description:     "description",
		HealthCheckPort: 12345,
	}, got)
}

func TestAdapter_GetElasticIP_mapsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

	_, err := NewAdapter(sdk).GetElasticIP(ctx, id)

	assert.ErrorIs(t, err, domain.ErrElasticIPNotFound)
}

func TestAdapter_ListElasticIPs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id1 := uuid.New()
	id2 := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListElasticIPS(ctx).Return(&egoscale.ListElasticIPSResponse{
		ElasticIPS: []egoscale.ElasticIP{
			{
				ID:          egoscale.UUID(id1.String()),
				IP:          "1.2.3.4",
				Description: "first",
				Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 6443},
			},
			{ID: egoscale.UUID(id2.String()), IP: "5.6.7.8", Description: "second"},
		},
	}, nil)

	got, err := NewAdapter(sdk).ListElasticIPs(ctx)

	assert.NoError(t, err)
	assert.Equal(t, []domain.ElasticIP{
		{ID: id1, IP: "1.2.3.4", Description: "first", HealthCheckPort: 6443},
		{ID: id2, IP: "5.6.7.8", Description: "second"},
	}, got)
}

func TestAdapter_ListElasticIPs_rejectsInvalidID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListElasticIPS(ctx).Return(&egoscale.ListElasticIPSResponse{
		ElasticIPS: []egoscale.ElasticIP{{ID: "not-a-uuid"}},
	}, nil)

	got, err := NewAdapter(sdk).ListElasticIPs(ctx)

	assert.Nil(t, got)
	assert.ErrorContains(t, err, "unable to parse elastic IP ID")
}

func TestAdapter_UpdateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	eip := domain.ElasticIP{ID: uuid.New(), Description: "description", HealthCheckPort: 12345}
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(eip.ID.String()), egoscale.UpdateElasticIPRequest{
		Description: eip.Description,
		Healthcheck: &egoscale.ElasticIPHealthcheck{
			Mode: egoscale.ElasticIPHealthcheckModeTCP,
			Port: int64(eip.HealthCheckPort),
		},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).UpdateElasticIP(ctx, eip))
}

func TestAdapter_DeleteElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).DeleteElasticIP(ctx, id))
}

func TestAdapter_CreateSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{Name: "sg-name"}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{
			State:     egoscale.OperationStateSuccess,
			Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())},
		}, nil)

	got, err := NewAdapter(sdk).CreateSecurityGroup(ctx, "sg-name")

	assert.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestAdapter_GetSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).Return(&egoscale.SecurityGroup{Name: "sg-name"}, nil)

	got, err := NewAdapter(sdk).GetSecurityGroup(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, domain.SecurityGroup{ID: id, Name: "sg-name"}, got)
}

func TestAdapter_GetSecurityGroup_mapsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

	_, err := NewAdapter(sdk).GetSecurityGroup(ctx, id)

	assert.ErrorIs(t, err, domain.ErrSecurityGroupNotFound)
}

func TestAdapter_DeleteSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).DeleteSecurityGroup(ctx, id))
}

func TestAdapter_CreateSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()
	sourceID := uuid.New()
	network := "10.0.0.0/8"
	rule := domain.SecurityGroupRule{
		Description:   "allow tcp",
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
		Network:       &network,
		SecurityGroup: &sourceID,
	}
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
		Description:   rule.Description,
		FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirectionIngress,
		Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocolTCP,
		StartPort:     rule.StartPort,
		EndPort:       rule.EndPort,
		Network:       network,
		SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sourceID.String())},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{
			State:     egoscale.OperationStateSuccess,
			Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())},
		}, nil)

	got, err := NewAdapter(sdk).CreateSecurityGroupRule(ctx, sgID, rule)

	assert.NoError(t, err)
	assert.Equal(t, ruleID, got)
}

func TestAdapter_DeleteSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).DeleteSecurityGroupRule(ctx, sgID, ruleID))
}

func TestAdapter_ListSecurityGroupRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID1 := uuid.New()
	ruleID2 := uuid.New()
	sourceID := uuid.New()
	network := "10.0.0.0/8"
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).Return(&egoscale.SecurityGroup{
		Rules: []egoscale.SecurityGroupRule{
			{
				ID:            egoscale.UUID(ruleID1.String()),
				Description:   "allow tcp",
				FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
				StartPort:     80,
				EndPort:       80,
				Network:       network,
			},
			{
				ID:            egoscale.UUID(ruleID2.String()),
				Description:   "allow from sg",
				FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
				Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
				StartPort:     443,
				EndPort:       443,
				SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sourceID.String())},
			},
		},
	}, nil)

	got, err := NewAdapter(sdk).ListSecurityGroupRules(ctx, sgID)

	assert.NoError(t, err)
	assert.Equal(t, []domain.SecurityGroupRule{
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
			SecurityGroup: &sourceID,
		},
	}, got)
}

func TestAdapter_ListSecurityGroupRules_mapsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).Return(nil, egoscale.ErrNotFound)

	_, err := NewAdapter(sdk).ListSecurityGroupRules(ctx, sgID)

	assert.ErrorIs(t, err, domain.ErrSecurityGroupNotFound)
}

func TestAdapter_ListSecurityGroupRules_rejectsInvalidIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()
	tests := []struct {
		name    string
		rule    egoscale.SecurityGroupRule
		wantErr string
	}{
		{name: "rule", rule: egoscale.SecurityGroupRule{ID: "not-a-uuid"}, wantErr: "unable to parse security group rule ID"},
		{
			name: "source security group",
			rule: egoscale.SecurityGroupRule{
				ID:            egoscale.UUID(ruleID.String()),
				SecurityGroup: &egoscale.SecurityGroupResource{ID: "not-a-uuid"},
			},
			wantErr: "unable to parse security group rule source ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sdk := mocks.NewExoscaleClient(t)
			sdk.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).Return(&egoscale.SecurityGroup{
				Rules: []egoscale.SecurityGroupRule{tc.rule},
			}, nil)

			got, err := NewAdapter(sdk).ListSecurityGroupRules(ctx, sgID)

			assert.Nil(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestAdapter_CreateInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	securityGroupID := uuid.New()
	instanceTypeID := egoscale.UUID(uuid.New().String())
	instanceType := egoscale.InstanceType{
		ID:     instanceTypeID,
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	spec := domain.ResolvedInstanceSpec{
		Name:             "machine-0",
		TemplateID:       templateID,
		InstanceType:     domain.InstanceType{ID: instanceTypeID.String(), Family: "standard", Size: "2"},
		SSHKey:           "ssh-key",
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
		DiskSizeGiB:      20,
		UserData:         "#cloud-config",
		Labels:           map[string]string{"machine": "uid"},
	}

	createOp := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateInstance(ctx, egoscale.CreateInstanceRequest{
		DiskSize:           int64(20),
		InstanceType:       &instanceType,
		Labels:             egoscale.Labels{"machine": "uid"},
		Name:               "machine-0",
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
		SSHKey:             &egoscale.SSHKey{Name: "ssh-key"},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
		UserData:           base64.StdEncoding.EncodeToString([]byte("#cloud-config")),
	}).Return(createOp, nil)
	sdk.EXPECT().Wait(ctx, createOp, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: createOp.Reference}, nil)

	got, err := NewAdapter(sdk).CreateInstance(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, instanceID, got)
}

func TestAdapter_CreateInstance_withoutOptionalFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	instanceTypeID := uuid.New().String()
	spec := domain.ResolvedInstanceSpec{
		TemplateID:   templateID,
		InstanceType: domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"},
		DiskSizeGiB:  10,
	}
	createOp := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateInstance(ctx, egoscale.CreateInstanceRequest{
		DiskSize: int64(10),
		InstanceType: &egoscale.InstanceType{
			ID:     egoscale.UUID(instanceTypeID),
			Family: egoscale.InstanceTypeFamilyStandard,
			Size:   egoscale.InstanceTypeSize("2"),
		},
		Labels:             egoscale.Labels(nil),
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
	}).Return(createOp, nil)
	sdk.EXPECT().Wait(ctx, createOp, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{
		State:     egoscale.OperationStateSuccess,
		Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())},
	}, nil)

	got, err := NewAdapter(sdk).CreateInstance(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, instanceID, got)
}

func TestAdapter_CreateInstance_returnsCreateError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	instanceTypeID := uuid.New().String()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().CreateInstance(ctx, egoscale.CreateInstanceRequest{
		DiskSize: int64(10),
		InstanceType: &egoscale.InstanceType{
			ID:     egoscale.UUID(instanceTypeID),
			Family: egoscale.InstanceTypeFamilyStandard,
			Size:   egoscale.InstanceTypeSize("2"),
		},
		Labels:             egoscale.Labels(nil),
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
	}).Return(nil, assert.AnError)

	got, err := NewAdapter(sdk).CreateInstance(ctx, domain.ResolvedInstanceSpec{
		TemplateID:   templateID,
		InstanceType: domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"},
		DiskSizeGiB:  10,
	})

	assert.Equal(t, uuid.Nil, got)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAdapter_ListInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	label := "machine=uid"
	id := uuid.New()
	securityGroupID := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	sdk := mocks.NewExoscaleClient(t)
	labelOption := mock.MatchedBy(func(opts []egoscale.ListInstancesOpt) bool {
		if len(opts) != 1 {
			return false
		}
		values := url.Values{}
		opts[0](values)
		return values.Get("labels") == label
	})
	sdk.EXPECT().ListInstances(ctx, labelOption).Return(&egoscale.ListInstancesResponse{
		Instances: []egoscale.ListInstancesResponseInstances{{
			ID:             egoscale.UUID(id.String()),
			Name:           "machine-0",
			State:          egoscale.InstanceStateRunning,
			PublicIP:       net.ParseIP("1.2.3.4"),
			CreatedAT:      createdAt,
			Labels:         egoscale.Labels{"machine": "uid"},
			SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
		}},
	}, nil)

	got, err := NewAdapter(sdk).ListInstances(ctx, label)

	assert.NoError(t, err)
	assert.Equal(t, []domain.Instance{{
		ID:               id,
		Name:             "machine-0",
		State:            "running",
		PublicIP:         "1.2.3.4",
		CreatedAt:        createdAt.Format(time.RFC3339),
		Labels:           map[string]string{"machine": "uid"},
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
	}}, got)
}

func TestAdapter_ListInstances_returnsListError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListInstances(ctx, mock.Anything).Return(nil, assert.AnError)

	got, err := NewAdapter(sdk).ListInstances(ctx, "machine=uid")

	assert.Nil(t, got)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAdapter_ListInstances_rejectsInvalidIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	tests := []struct {
		name     string
		instance egoscale.ListInstancesResponseInstances
		wantErr  string
	}{
		{name: "instance", instance: egoscale.ListInstancesResponseInstances{ID: "not-a-uuid"}, wantErr: "unable to parse instance ID"},
		{
			name: "security group",
			instance: egoscale.ListInstancesResponseInstances{
				ID:             egoscale.UUID(id.String()),
				SecurityGroups: []egoscale.SecurityGroup{{ID: "not-a-uuid"}},
			},
			wantErr: "unable to parse instance security group ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sdk := mocks.NewExoscaleClient(t)
			sdk.EXPECT().ListInstances(ctx, mock.Anything).Return(&egoscale.ListInstancesResponse{
				Instances: []egoscale.ListInstancesResponseInstances{tc.instance},
			}, nil)

			got, err := NewAdapter(sdk).ListInstances(ctx, "machine=uid")

			assert.Nil(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestAdapter_GetInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	securityGroupID := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(&egoscale.Instance{
		Name:           "machine-0",
		State:          egoscale.InstanceStateRunning,
		PublicIP:       net.ParseIP("1.2.3.4"),
		CreatedAT:      createdAt,
		Labels:         egoscale.Labels{"machine": "uid"},
		SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
	}, nil)

	got, err := NewAdapter(sdk).GetInstance(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, domain.Instance{
		ID:               id,
		Name:             "machine-0",
		State:            "running",
		PublicIP:         "1.2.3.4",
		CreatedAt:        createdAt.Format(time.RFC3339),
		Labels:           map[string]string{"machine": "uid"},
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
	}, got)
}

func TestAdapter_GetInstance_mapsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

	_, err := NewAdapter(sdk).GetInstance(ctx, id)

	assert.ErrorIs(t, err, domain.ErrInstanceNotFound)
}

func TestAdapter_GetInstance_rejectsInvalidSecurityGroupID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(&egoscale.Instance{
		SecurityGroups: []egoscale.SecurityGroup{{ID: "not-a-uuid"}},
	}, nil)

	got, err := NewAdapter(sdk).GetInstance(ctx, id)

	assert.Equal(t, domain.Instance{}, got)
	assert.ErrorContains(t, err, "unable to parse instance security group ID")
}

func TestAdapter_ListInstanceTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListInstanceTypes(ctx).Return(&egoscale.ListInstanceTypesResponse{
		InstanceTypes: []egoscale.InstanceType{{
			ID:     egoscale.UUID(id.String()),
			Family: egoscale.InstanceTypeFamilyStandard,
			Size:   egoscale.InstanceTypeSize("2"),
		}},
	}, nil)

	got, err := NewAdapter(sdk).ListInstanceTypes(ctx)

	assert.NoError(t, err)
	assert.Equal(t, []domain.InstanceType{{ID: id.String(), Family: "standard", Size: "2"}}, got)
}

func TestAdapter_GetTemplate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	createdAt := time.Now().UTC()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().GetTemplate(ctx, egoscale.UUID(templateID.String())).Return(&egoscale.Template{
		ID:        egoscale.UUID(templateID.String()),
		Name:      "ubuntu",
		Size:      15,
		CreatedAT: createdAt,
	}, nil)

	got, err := NewAdapter(sdk).GetTemplate(ctx, templateID)

	assert.NoError(t, err)
	assert.Equal(t, domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15, CreatedAt: createdAt}, got)
}

func TestAdapter_ListTemplates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	createdAt := time.Now().UTC()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListTemplates(ctx).Return(&egoscale.ListTemplatesResponse{
		Templates: []egoscale.Template{{
			ID:        egoscale.UUID(templateID.String()),
			Name:      "ubuntu",
			Size:      15,
			CreatedAT: createdAt,
		}},
	}, nil)

	got, err := NewAdapter(sdk).ListTemplates(ctx)

	assert.NoError(t, err)
	assert.Equal(t, []domain.InstanceTemplate{{ID: templateID, Name: "ubuntu", SizeBytes: 15, CreatedAt: createdAt}}, got)
}

func TestAdapter_ListTemplates_rejectsInvalidID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().ListTemplates(ctx).Return(&egoscale.ListTemplatesResponse{
		Templates: []egoscale.Template{{ID: "not-a-uuid"}},
	}, nil)

	got, err := NewAdapter(sdk).ListTemplates(ctx)

	assert.Nil(t, got)
	assert.ErrorContains(t, err, `unable to parse template id "not-a-uuid"`)
}

func TestAdapter_AttachInstanceToElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	elasticIPID := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), egoscale.AttachInstanceToElasticIPRequest{
		Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).AttachInstanceToElasticIP(ctx, instanceID, elasticIPID))
}

func TestAdapter_AttachInstanceToSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.AttachInstanceToSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID))
}

func TestAdapter_DetachInstanceFromSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), egoscale.DetachInstanceFromSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	}).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).DetachInstanceFromSecurityGroup(ctx, instanceID, securityGroupID))
}

func TestAdapter_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{}
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(op, nil)
	sdk.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

	assert.NoError(t, NewAdapter(sdk).DeleteInstance(ctx, id))
}

func TestAdapter_DeleteInstance_mapsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	sdk := mocks.NewExoscaleClient(t)
	sdk.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

	assert.ErrorIs(t, NewAdapter(sdk).DeleteInstance(ctx, id), domain.ErrInstanceNotFound)
}

func TestAdapter_waitForSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := &egoscale.Operation{State: egoscale.OperationStatePending}
	tests := []struct {
		name      string
		completed *egoscale.Operation
		waitErr   error
		wantErr   string
	}{
		{name: "success", completed: &egoscale.Operation{State: egoscale.OperationStateSuccess}},
		{name: "wait error", waitErr: assert.AnError, wantErr: assert.AnError.Error()},
		{name: "nil completion", wantErr: "operation did not succeed"},
		{name: "failure completion", completed: &egoscale.Operation{State: egoscale.OperationStateFailure}, wantErr: "operation did not succeed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mocks.NewExoscaleClient(t)
			client.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(tc.completed, tc.waitErr)

			completed, err := NewAdapter(client).waitForSuccess(ctx, op)

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, completed)
			} else {
				assert.NoError(t, err)
				assert.Same(t, tc.completed, completed)
			}
		})
	}
}

func TestOperationReferenceID(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	tests := []struct {
		name      string
		operation *egoscale.Operation
		want      uuid.UUID
		wantErr   string
	}{
		{name: "success", operation: &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}}, want: id},
		{name: "missing reference", operation: &egoscale.Operation{}, wantErr: "operation returned no reference"},
		{name: "malformed UUID", operation: &egoscale.Operation{Reference: &egoscale.OperationReference{ID: "not-a-uuid"}}, wantErr: "invalid UUID"},
		{name: "zero UUID", operation: &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(uuid.Nil.String())}}, wantErr: "operation reference ID is nil"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := operationReferenceID(tc.operation)

			assert.Equal(t, tc.want, got)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
