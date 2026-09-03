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
	workerSG := domain.SecurityGroup{
		ID:   uuid.New(),
		Name: fmt.Sprintf("capi - %s - worker", clusterID.String()),
	}

	desiredCPRules, _ := resolveRules(cpSG.ID, workerSG.ID, defaultControlPlaneRules(port), nil)
	desiredWorkerRulesList, _ := resolveRules(cpSG.ID, workerSG.ID, defaultWorkerRules(), nil)

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
	workerRules := []domain.SecurityGroupRule{
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:   workerSG.ID.String(),
						Name: workerSG.Name,
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
					UpsertSecurityGroup(ctx, clusterID, &workerSG.ID, workerSG.Name).
					Return(workerSG, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, cpSG.ID, desiredCPRules).
					Return(cpRules, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, workerSG.ID, desiredWorkerRulesList).
					Return(workerRules, nil)
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:    workerSG.ID.String(),
						Name:  workerSG.Name,
						Rules: domainRulesToStatus(workerRules),
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
			name: "UpsertSecurityGroup worker error",
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
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), workerSG.Name).
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
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), workerSG.Name).
					Return(workerSG, nil)
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:   workerSG.ID.String(),
						Name: workerSG.Name,
					},
				},
			},
			err: assert.AnError,
		},
		{
			name: "UpsertSecurityGroupRules worker error",
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
					UpsertSecurityGroup(ctx, clusterID, (*uuid.UUID)(nil), workerSG.Name).
					Return(workerSG, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, cpSG.ID, desiredCPRules).
					Return(cpRules, nil)
				m.EXPECT().
					UpsertSecurityGroupRules(ctx, workerSG.ID, desiredWorkerRulesList).
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:   workerSG.ID.String(),
						Name: workerSG.Name,
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
	workerSG := domain.SecurityGroup{
		Name: fmt.Sprintf("capi - %s - worker", clusterID.String()),
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
			name: "invalid worker SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Spec: infrav1alpha1.ExoscaleClusterSpec{
					ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{Port: port},
				},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID: new(clusterID.String()),
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: workerSG.Name,
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID:   "not-a-uuid",
						Name: workerSG.Name,
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
	workerSGID := uuid.New()

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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
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
					PurgeSecurityGroup(ctx, workerSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, cpSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, workerSGID).
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
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
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
			name: "invalid worker SG ID",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: "not-a-uuid",
					},
				},
			},
			err: errInvalidID,
		},
		{
			name: "PurgeSecurityGroup worker error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, workerSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
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
			name: "DeleteSecurityGroup worker error",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
					},
				},
			},
			sgSvc: func(m *mocks.SecurityGroupService) {
				m.EXPECT().
					PurgeSecurityGroup(ctx, workerSGID).
					Return(nil)
				m.EXPECT().
					DeleteSecurityGroup(ctx, workerSGID).
					Return(assert.AnError)
			},
			output: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					SecurityGroupWorker: &infrav1alpha1.SecurityGroupStatus{
						ID: workerSGID.String(),
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

func Test_clusterService_DeleteCluster_recoversMovedResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clusterID := uuid.New()
	eip := domain.ElasticIP{ID: uuid.New()}
	controlPlane := domain.SecurityGroup{ID: uuid.New(), Name: fmt.Sprintf("capi - %s - control plane", clusterID)}
	worker := domain.SecurityGroup{ID: uuid.New(), Name: fmt.Sprintf("capi - %s - worker", clusterID)}
	cluster := infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{
		Name:        "moved-cluster",
		Annotations: map[string]string{domain.ClusterIDKey: clusterID.String()},
	}}

	eipSvc := mocks.NewElasticIPService(t)
	eipSvc.EXPECT().FindElasticIP(ctx, clusterID, (*uuid.UUID)(nil)).Return(eip, nil)
	eipSvc.EXPECT().DeleteElasticIP(ctx, eip.ID).Return(nil)
	sgSvc := mocks.NewSecurityGroupService(t)
	sgSvc.EXPECT().FindSecurityGroup(ctx, clusterID, controlPlane.Name, (*uuid.UUID)(nil)).Return(controlPlane, nil)
	sgSvc.EXPECT().FindSecurityGroup(ctx, clusterID, worker.Name, (*uuid.UUID)(nil)).Return(worker, nil)
	sgSvc.EXPECT().PurgeSecurityGroup(ctx, controlPlane.ID).Return(nil)
	sgSvc.EXPECT().PurgeSecurityGroup(ctx, worker.ID).Return(nil)
	sgSvc.EXPECT().DeleteSecurityGroup(ctx, controlPlane.ID).Return(nil)
	sgSvc.EXPECT().DeleteSecurityGroup(ctx, worker.ID).Return(nil)

	got, err := (&clusterService{elasticIPSvc: eipSvc, securityGroupSvc: sgSvc}).DeleteCluster(ctx, cluster)

	assert.NoError(t, err)
	assert.Equal(t, cluster, got)
}

func Test_clusterService_DeleteCluster_usesOwnedStatusResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clusterID := uuid.New()
	eip := domain.ElasticIP{ID: uuid.New()}
	controlPlane := domain.SecurityGroup{ID: uuid.New(), Name: fmt.Sprintf("capi - %s - control plane", clusterID)}
	worker := domain.SecurityGroup{ID: uuid.New(), Name: fmt.Sprintf("capi - %s - worker", clusterID)}
	cluster := infrav1alpha1.ExoscaleCluster{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: clusterID.String()}},
		Status: infrav1alpha1.ExoscaleClusterStatus{
			ID:                       new(clusterID.String()),
			ControlPlaneEndpoint:     &infrav1alpha1.APIEndpointStatus{ID: eip.ID.String()},
			SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: controlPlane.ID.String()},
			SecurityGroupWorker:      &infrav1alpha1.SecurityGroupStatus{ID: worker.ID.String()},
		},
	}

	eipSvc := mocks.NewElasticIPService(t)
	eipSvc.EXPECT().FindElasticIP(ctx, clusterID, &eip.ID).Return(eip, nil)
	eipSvc.EXPECT().DeleteElasticIP(ctx, eip.ID).Return(nil)
	sgSvc := mocks.NewSecurityGroupService(t)
	sgSvc.EXPECT().FindSecurityGroup(ctx, clusterID, controlPlane.Name, &controlPlane.ID).Return(controlPlane, nil)
	sgSvc.EXPECT().FindSecurityGroup(ctx, clusterID, worker.Name, &worker.ID).Return(worker, nil)
	sgSvc.EXPECT().PurgeSecurityGroup(ctx, controlPlane.ID).Return(nil)
	sgSvc.EXPECT().PurgeSecurityGroup(ctx, worker.ID).Return(nil)
	sgSvc.EXPECT().DeleteSecurityGroup(ctx, controlPlane.ID).Return(nil)
	sgSvc.EXPECT().DeleteSecurityGroup(ctx, worker.ID).Return(nil)

	_, err := (&clusterService{elasticIPSvc: eipSvc, securityGroupSvc: sgSvc}).DeleteCluster(ctx, cluster)
	assert.NoError(t, err)
}

func Test_clusterOwnershipID(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()
	tests := []struct {
		name       string
		cluster    infrav1alpha1.ExoscaleCluster
		want       string
		wantErrMsg string
	}{
		{name: "annotation", cluster: infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: id}}}, want: id},
		{name: "legacy status", cluster: infrav1alpha1.ExoscaleCluster{Status: infrav1alpha1.ExoscaleClusterStatus{ID: &id}}, want: id},
		{
			name: "matching annotation and status",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: id}},
				Status:     infrav1alpha1.ExoscaleClusterStatus{ID: &id},
			},
			want: id,
		},
		{
			name: "conflict",
			cluster: infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: id}},
				Status:     infrav1alpha1.ExoscaleClusterStatus{ID: new(uuid.NewString())},
			},
			wantErrMsg: "does not match status",
		},
		{name: "invalid", cluster: infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: "bad"}}}, wantErrMsg: "invalid id"},
		{name: "missing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := clusterOwnershipID(tc.cluster)

			if tc.wantErrMsg != "" {
				assert.ErrorContains(t, err, tc.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			if tc.want == "" {
				assert.Nil(t, got)
			} else if assert.NotNil(t, got) {
				assert.Equal(t, tc.want, got.String())
			}
		})
	}
}
