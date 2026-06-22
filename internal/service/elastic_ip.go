package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

var _ domain.ElasticIPService = (*elasticIPService)(nil)

type elasticIPService struct {
	cloud  domain.Cloud
	logger logr.Logger
}

func NewElasticIPService(client domain.Cloud, logger logr.Logger) *elasticIPService {
	return &elasticIPService{cloud: client, logger: logger}
}

func (s *elasticIPService) UpsertElasticIP(ctx context.Context, clusterID uuid.UUID, eipID *uuid.UUID, port int32) (domain.ElasticIP, error) {
	eipDescription := fmt.Sprintf("capi - clusterID - %s", clusterID.String())

	if eipID != nil {
		eip, err := s.cloud.GetElasticIP(ctx, *eipID)
		if err != nil && !errors.Is(err, domain.ErrElasticIPNotFound) {
			return domain.ElasticIP{}, fmt.Errorf("error while fetching eip: %w", err)
		} else if errors.Is(err, domain.ErrElasticIPNotFound) {
			s.logger.Info("Elastic IP not found, will create a new one")

			id, err := s.cloud.CreateElasticIP(ctx, port, eipDescription)
			if err != nil {
				return domain.ElasticIP{}, fmt.Errorf("error while creating eip: %w", err)
			}
			eip, err = s.cloud.GetElasticIP(ctx, id)
			if err != nil {
				return domain.ElasticIP{}, fmt.Errorf("error while fetching new eip, %w", err)
			}
		}

		if eip.Description != eipDescription || port != eip.HealthCheckPort {
			s.logger.Info("Update elastic IP")

			eip.Description = eipDescription
			eip.HealthCheckPort = port
			if err := s.cloud.UpdateElasticIP(ctx, eip); err != nil {
				return domain.ElasticIP{}, fmt.Errorf("error while updating the eip: %w", err)
			}
		}
		return eip, nil
	} else {
		s.logger.Info("Create elastic IP")

		id, err := s.cloud.CreateElasticIP(ctx, port, eipDescription)
		if err != nil {
			return domain.ElasticIP{}, fmt.Errorf("error while creating eip: %w", err)
		}
		eip, err := s.cloud.GetElasticIP(ctx, id)
		if err != nil {
			return domain.ElasticIP{}, fmt.Errorf("error while fetching new eip, %w", err)
		}
		return eip, nil
	}
}

func (s elasticIPService) DeleteElasticIP(ctx context.Context, id uuid.UUID) error {
	if _, err := s.cloud.GetElasticIP(ctx, id); err != nil {
		if errors.Is(err, domain.ErrElasticIPNotFound) {
			return nil
		}
		return err
	}

	s.logger.Info("Delete elastic IP")
	if err := s.cloud.DeleteElasticIP(ctx, id); err != nil {
		return err
	}

	return nil
}
