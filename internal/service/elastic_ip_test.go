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
			name: "nominal - no eip",
			port: port,
			cloud: func(m *mocks.Cloud) {
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
			name:  "nominal - eip not found",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				mock.InOrder(
					m.EXPECT().
						GetElasticIP(ctx, eipID).
						Return(domain.ElasticIP{}, domain.ErrElasticIPNotFound).
						Once(),
					m.EXPECT().
						CreateElasticIP(ctx, port, description).
						Return(eipID, nil).
						Once(),
					m.EXPECT().
						GetElasticIP(ctx, eipID).Return(
						domain.ElasticIP{
							ID:              eipID,
							Description:     description,
							HealthCheckPort: port,
							IP:              "1.2.3.4",
						}, nil).
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
			name:  "nominal - eip found - need update",
			eipID: &eipID,
			port:  port,
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().
					GetElasticIP(ctx, eipID).Return(
					domain.ElasticIP{
						ID:              eipID,
						Description:     "diff in description",
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
						Description:     "diff in description",
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
			name: "create eip returned an error",
			port: port,
			cloud: func(m *mocks.Cloud) {
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

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_elasticIPService_DeleteElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name  string
		cloud func(m *mocks.Cloud)
		err   error
	}{
		{
			name: "nominal - eip found",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{}, nil)
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
			name: "get eip returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "delete eip returned an error",
			cloud: func(m *mocks.Cloud) {
				m.EXPECT().GetElasticIP(ctx, id).Return(domain.ElasticIP{}, nil)
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

			err := svc.DeleteElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}
