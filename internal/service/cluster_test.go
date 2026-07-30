package service

import (
	"context"
	"fmt"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_clusterService_ReconcileCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	clusterID := uuid.New()
	port := int32(6443)

	eip := domain.ElasticIP{
		ID:              uuid.New(),
		IP:              "1.2.3.4",
		Description:     fmt.Sprintf("capi - clusterID - %s", clusterID.String()),
		HealthCheckPort: port,
	}
	cpSG := domain.SecurityGroup{
		ID:   uuid.New(),
		Name: fmt.Sprintf("capi - %s - control plane", clusterID.String()),
	}
	nodeSG := domain.SecurityGroup{
		ID:   uuid.New(),
		Name: fmt.Sprintf("capi - %s - node", clusterID.String()),
	}

	desiredCPRules, _ := desiredControlPlaneRules(cpSG.ID, nodeSG.ID, port, nil)
	desiredNodeRulesList, _ := desiredNodeRules(cpSG.ID, nodeSG.ID, nil)

	cpRules := []domain.SecurityGroupRule{
		{
			ID:            uuid.New(),
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     int64(port),
			EndPort:       int64(port),
			Network:       new("0.0.0.0/0"),
		},
	}
	nodeRules := []domain.SecurityGroupRule{
		{
			ID:            uuid.New(),
			FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      domain.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: new(uuid.New()),
		},
	}

	tests := []struct {
		name    string
		cluster infrav1alpha1.ExoscaleCluster
		eipSvc  func(m *mocks.ElasticIPService)
		sgSvc   func(m *mocks.SecurityGroupService)
		output  infrav1alpha1.ExoscaleCluster
		err     error
	}{
		{
			name: "nominal",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "old-ip", Port: port},
						ID:          eip.ID.String(),
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   cpSG.ID.String(),
						Name: cpSG.Name,
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:   nodeSG.ID.String(),
						Name: nodeSG.Name,
					},
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, &eip.ID, port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, &cpSG.ID, cpSG.Name).
					Return(cpSG, nil)
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, &nodeSG.ID, nodeSG.Name).
					Return(nodeSG, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, cpSG.ID, desiredCPRules).
					Return(cpRules, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, nodeSG.ID, desiredNodeRulesList).
					Return(nodeRules, nil)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:    cpSG.ID.String(),
						Name:  cpSG.Name,
						Rules: domainRulesToStatus(cpRules),
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:    nodeSG.ID.String(),
						Name:  nodeSG.Name,
						Rules: domainRulesToStatus(nodeRules),
					},
					Initialization: infrav1alpha1.ExoscaleClusterInitializationStatus{
						Provisioned: new(true),
					},
				},
			},
		},
		{
			name: "UpsertElasticIP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(domain.ElasticIP{}, assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			err: assert.AnError,
		},
		{
			name: "UpsertSecurityGroup CP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), cpSG.Name).
					Return(domain.SecurityGroup{}, assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "UpsertSecurityGroup node error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), cpSG.Name).
					Return(cpSG, nil)
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), nodeSG.Name).
					Return(domain.SecurityGroup{}, assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   cpSG.ID.String(),
						Name: cpSG.Name,
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "UpsertSecurityGroupRules CP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), cpSG.Name).
					Return(cpSG, nil)
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), nodeSG.Name).
					Return(nodeSG, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, cpSG.ID, desiredCPRules).
					Return(nil, assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   cpSG.ID.String(),
						Name: cpSG.Name,
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:   nodeSG.ID.String(),
						Name: nodeSG.Name,
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "UpsertSecurityGroupRules node error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), cpSG.Name).
					Return(cpSG, nil)
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), nodeSG.Name).
					Return(nodeSG, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, cpSG.ID, desiredCPRules).
					Return(cpRules, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, nodeSG.ID, desiredNodeRulesList).
					Return(nil, assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:    cpSG.ID.String(),
						Name:  cpSG.Name,
						Rules: domainRulesToStatus(cpRules),
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:   nodeSG.ID.String(),
						Name: nodeSG.Name,
					},
				},
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			eipSvc := mocks.NewElasticIPService(t)
			if ut.eipSvc != nil {
				ut.eipSvc(eipSvc)
			}

			sgSvc := mocks.NewSecurityGroupService(t)
			if ut.sgSvc != nil {
				ut.sgSvc(sgSvc)
			}

			svc := clusterService{
				elasticIPSvc:     eipSvc,
				securityGroupSvc: sgSvc,
				logger:           logr.Discard(),
			}

			output, err := svc.ReconcileCluster(ctx, ut.cluster)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_clusterService_ReconcileCluster_invalid_id(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	clusterID := uuid.New()
	port := int32(6443)

	eip := domain.ElasticIP{
		ID:              uuid.New(),
		IP:              "1.2.3.4",
		Description:     fmt.Sprintf("capi - clusterID - %s", clusterID.String()),
		HealthCheckPort: port,
	}
	cpSG := domain.SecurityGroup{
		ID:   uuid.New(),
		Name: fmt.Sprintf("capi - %s - control plane", clusterID.String()),
	}
	nodeSG := domain.SecurityGroup{
		Name: fmt.Sprintf("capi - %s - node", clusterID.String()),
	}

	tests := []struct {
		name    string
		cluster infrav1alpha1.ExoscaleCluster
		eipSvc  func(m *mocks.ElasticIPService)
		sgSvc   func(m *mocks.SecurityGroupService)
		output  infrav1alpha1.ExoscaleCluster
		err     error
	}{
		{
			name: "invalid cluster ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new("not-a-uuid"),
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new("not-a-uuid"),
				},
			},
			err: errInvalidID,
		},
		{
			name: "invalid EIP ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Port: port},
						ID:          "not-a-uuid",
					},
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Port: port},
						ID:          "not-a-uuid",
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "invalid CP SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: cpSG.Name,
					},
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: cpSG.Name,
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "invalid node SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: nodeSG.Name,
					},
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					UpsertElasticIP(ctx, clusterID, (*uuid.UUID)(nil), port).
					Return(eip, nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), cpSG.Name).
					Return(cpSG, nil)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						APIEndpoint: infrav1alpha1.APIEndpoint{Host: "1.2.3.4", Port: port},
						ID:          eip.ID.String(),
						Description: eip.Description,
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID:   cpSG.ID.String(),
						Name: cpSG.Name,
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: nodeSG.Name,
					},
				},
			},
			err: errInvalidID,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			eipSvc := mocks.NewElasticIPService(t)
			if ut.eipSvc != nil {
				ut.eipSvc(eipSvc)
			}

			sgSvc := mocks.NewSecurityGroupService(t)
			if ut.sgSvc != nil {
				ut.sgSvc(sgSvc)
			}

			svc := clusterService{
				elasticIPSvc:     eipSvc,
				securityGroupSvc: sgSvc,
				logger:           logr.Discard(),
			}

			output, err := svc.ReconcileCluster(ctx, ut.cluster)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_clusterService_DeleteCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	eipID := uuid.New()
	cpSGID := uuid.New()
	nodeSGID := uuid.New()

	tests := []struct {
		name    string
		cluster infrav1alpha1.ExoscaleCluster
		eipSvc  func(m *mocks.ElasticIPService)
		sgSvc   func(m *mocks.SecurityGroupService)
		output  infrav1alpha1.ExoscaleCluster
		err     error
	}{
		{
			name: "nominal",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: eipID.String(),
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					DeleteElasticIP(ctx, eipID).
					Return(nil)
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, cpSGID).
					Return(nil)
				m.EXPECT().
					PurgeSecurityGroup(ctx, nodeSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, cpSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, nodeSGID).
					Return(nil)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: eipID.String(),
					},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
		},
		{
			name: "nominal no status",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			},
		},
		{
			name: "invalid EIP ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: "not-a-uuid",
					},
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: "not-a-uuid",
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "DeleteElasticIP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: eipID.String(),
					},
				},
			},
			eipSvc: func(m *mocks.ElasticIPService) {
				m.EXPECT().
					DeleteElasticIP(ctx, eipID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ControlPlaneEndpoint: &infrav1alpha1.APIEndpointStatus{
						ID: eipID.String(),
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "invalid CP SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "PurgeSecurityGroup CP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, cpSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "invalid node SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "PurgeSecurityGroup node error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, nodeSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "DeleteSecurityGroup CP error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, cpSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, cpSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{
						ID: cpSGID.String(),
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "DeleteSecurityGroup node error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, nodeSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, nodeSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{
						ID: nodeSGID.String(),
					},
				},
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			eipSvc := mocks.NewElasticIPService(t)
			if ut.eipSvc != nil {
				ut.eipSvc(eipSvc)
			}

			sgSvc := mocks.NewSecurityGroupService(t)
			if ut.sgSvc != nil {
				ut.sgSvc(sgSvc)
			}

			svc := clusterService{
				elasticIPSvc:     eipSvc,
				securityGroupSvc: sgSvc,
				logger:           logr.Discard(),
			}

			output, err := svc.DeleteCluster(ctx, ut.cluster)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}
