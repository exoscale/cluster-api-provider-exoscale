package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_elasticIPService_UpsertElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clusterID := uuid.New()
	port := int32(12345)
	description := fmt.Sprintf("capi - clusterID - %s", clusterID.String())
	eipID := uuid.New()

	tests := []struct {
		name   string
		eipID  *uuid.UUID
		port   int32
		cloud  func(m *mocks.Cloud)
		output domain.ElasticIP
		err    error
	}{
		{
			name: "nominal - no eip - not found in cloud - creates new one",
			port: port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListElasticIPs(ctx).
					Return([]domain.ElasticIP{}, nil)
				m.EXPECT().
					CreateElasticIP(ctx, port, description).
					Return(eipID, nil)
				m.EXPECT().
					GetElasticIP(ctx, eipID).Return(
					domain.ElasticIP{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: port,
						IP:              "1.2.3.4",
					}, nil)
			},
			output: domain.ElasticIP{
				ID:              eipID,
				Description:     description,
				HealthCheckPort: port,
				IP:              "1.2.3.4",
			},
		},
		{
			name: "nominal - no eip - found by cluster ID and reused",
			port: port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListElasticIPs(ctx).
					Return([]domain.ElasticIP{{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: port,
						IP:              "1.2.3.4",
					}}, nil)
			},
			output: domain.ElasticIP{
				ID:              eipID,
				Description:     description,
				HealthCheckPort: port,
				IP:              "1.2.3.4",
			},
		},
		{
			name:  "stale status ID recovers existing elastic IP by description",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						GetElasticIP(ctx, eipID).
						Return(domain.ElasticIP{}, domain.ErrElasticIPNotFound).
						Once(),
					m.EXPECT().
						ListElasticIPs(ctx).Return([]domain.ElasticIP{{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: port,
						IP:              "1.2.3.4",
					}}, nil).
						Once(),
				)
			},
			output: domain.ElasticIP{
				ID:              eipID,
				Description:     description,
				HealthCheckPort: port,
				IP:              "1.2.3.4",
			},
		},
		{
			name:  "nominal - eip found - reconciles health-check port",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetElasticIP(ctx, eipID).Return(
					domain.ElasticIP{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: 0,
						IP:              "1.2.3.4",
					}, nil)
				m.EXPECT().UpdateElasticIP(ctx, domain.ElasticIP{
					ID:              eipID,
					Description:     description,
					HealthCheckPort: port,
					IP:              "1.2.3.4",
				}).Return(nil)
			},
			output: domain.ElasticIP{
				ID:              eipID,
				Description:     description,
				HealthCheckPort: port,
				IP:              "1.2.3.4",
			},
		},
		{
			name:  "rejects status elastic IP owned by another cluster",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, eipID).Return(domain.ElasticIP{
					ID: eipID, Description: "foreign", HealthCheckPort: port,
				}, nil)
			},
			err: errNotOwned,
		},
		{
			name: "description recovery reconciles health-check port",
			port: port,
			cloud: func(m *mocks.Cloud) {
				eip := domain.ElasticIP{ID: eipID, Description: description, HealthCheckPort: 1, IP: "1.2.3.4"}
				m.EXPECT().ListElasticIPs(ctx).Return([]domain.ElasticIP{eip}, nil)
				eip.HealthCheckPort = port
				m.EXPECT().UpdateElasticIP(ctx, eip).Return(nil)
			},
			output: domain.ElasticIP{ID: eipID, Description: description, HealthCheckPort: port, IP: "1.2.3.4"},
		},
		{
			name:  "nominal - eip found - no update",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetElasticIP(ctx, eipID).Return(
					domain.ElasticIP{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: port,
						IP:              "1.2.3.4",
					}, nil)
			},
			output: domain.ElasticIP{
				ID:              eipID,
				Description:     description,
				HealthCheckPort: port,
				IP:              "1.2.3.4",
			},
		},

		{
			name:  "get eip returned an error",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetElasticIP(ctx, eipID).
					Return(domain.ElasticIP{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name:  "update eip returned an error",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetElasticIP(ctx, eipID).Return(
					domain.ElasticIP{
						ID:              eipID,
						Description:     description,
						HealthCheckPort: 0,
						IP:              "1.2.3.4",
					}, nil)
				m.EXPECT().UpdateElasticIP(ctx, domain.ElasticIP{
					ID:              eipID,
					Description:     description,
					HealthCheckPort: port,
					IP:              "1.2.3.4",
				}).Return(assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "list eips returned an error",
			port: port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListElasticIPs(ctx).
					Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "create eip returned an error",
			port: port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					ListElasticIPs(ctx).
					Return([]domain.ElasticIP{}, nil)
				m.EXPECT().
					CreateElasticIP(ctx, port, description).
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

			svc := elasticIPService{cloud: cloud, logger: logr.Discard()}

			output, err := svc.UpsertElasticIP(ctx, clusterID, ut.eipID, port)

			if ut.err == errNotOwned {
				assert.ErrorContains(t, err, "is not owned by cluster")
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
			assert.Equal(t, ut.output, output)
		})
	}

}

func Test_elasticIPService_DeleteElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	clusterID := uuid.New()
	description := fmt.Sprintf("capi - clusterID - %s", clusterID)

	tests := []struct {
		name  string
		cloud func(m *mocks.Cloud)
		err   error
	}{
		{
			name: "nominal - eip found",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{ID: id, Description: description}, nil)
				m.EXPECT().DeleteElasticIP(ctx, id).Return(nil)
			},
		},
		{
			name: "nominal - eip not found",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{}, domain.ErrElasticIPNotFound)
			},
		},

		{
			name: "rejects foreign elastic IP",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{ID: id, Description: "foreign"}, nil)
			},
			err: errNotOwned,
		},
		{
			name: "get eip returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "delete eip returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{ID: id, Description: description}, nil)
				m.EXPECT().DeleteElasticIP(ctx, id).Return(assert.AnError)
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

			svc := elasticIPService{cloud: cloud, logger: logr.Discard()}

			err := svc.DeleteElasticIP(ctx, id, clusterID)

			if ut.err == errNotOwned {
				assert.ErrorContains(t, err, "is not owned by cluster")
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
		})
	}
}

func Test_elasticIPService_findElasticIPByClusterID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clusterID := uuid.New()
	matchingEIP := domain.ElasticIP{
		ID:          uuid.New(),
		IP:          "1.2.3.4",
		Description: fmt.Sprintf("capi - clusterID - %s", clusterID.String()),
	}
	otherEIP := domain.ElasticIP{
		ID:          uuid.New(),
		IP:          "5.6.7.8",
		Description: matchingEIP.Description + "-other",
	}

	tests := []struct {
		name   string
		cloud  func(m *mocks.Cloud)
		output domain.ElasticIP
		err    error
	}{
		{
			name: "found by cluster ID in description",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListElasticIPs(ctx).Return([]domain.ElasticIP{otherEIP, matchingEIP}, nil)
			},
			output: matchingEIP,
		},
		{
			name: "not found returns ErrElasticIPNotFound",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListElasticIPs(ctx).Return([]domain.ElasticIP{otherEIP}, nil)
			},
			err: domain.ErrElasticIPNotFound,
		},
		{
			name: "empty list returns ErrElasticIPNotFound",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListElasticIPs(ctx).Return([]domain.ElasticIP{}, nil)
			},
			err: domain.ErrElasticIPNotFound,
		},
		{
			name: "list error is propagated",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().ListElasticIPs(ctx).Return(nil, assert.AnError)
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

			svc := elasticIPService{cloud: cloud, logger: logr.Discard()}

			output, err := svc.FindElasticIP(ctx, clusterID)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}

	t.Run("rejects duplicate matching elastic IPs", func(t *testing.T) {
		cloud := mocks.NewCloud(t)
		cloud.EXPECT().ListElasticIPs(ctx).Return([]domain.ElasticIP{matchingEIP, {ID: uuid.New(), Description: matchingEIP.Description}}, nil)
		svc := elasticIPService{cloud: cloud, logger: logr.Discard()}

		_, err := svc.FindElasticIP(ctx, clusterID)

		assert.ErrorContains(t, err, "multiple elastic IPs")
	})
}
