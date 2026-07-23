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
		if err == nil {
			if eip.Description != eipDescription {
				return domain.ElasticIP{}, fmt.Errorf("elastic IP %s is not owned by cluster %s: description is %q", eip.ID, clusterID, eip.Description)
			}
			return s.reconcileHealthCheckPort(ctx, eip, port)
		}
		if !errors.Is(err, domain.ErrElasticIPNotFound) {
			return domain.ElasticIP{}, fmt.Errorf("error while fetching eip: %w", err)
		}
		s.logger.Info("Status elastic IP not found, recovering by description")
	}

	// Search before creating both on initial reconciliation and when status is stale.
	existing, err := s.FindElasticIP(ctx, clusterID)
	if err != nil && !errors.Is(err, domain.ErrElasticIPNotFound) {
		return domain.ElasticIP{}, fmt.Errorf("error while searching for existing eip: %w", err)
	} else if err == nil {
		s.logger.Info("Found an existing elastic IP by cluster ID, reusing it")
		return s.reconcileHealthCheckPort(ctx, existing, port)
	}

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

func (s *elasticIPService) reconcileHealthCheckPort(ctx context.Context, eip domain.ElasticIP, port int32) (domain.ElasticIP, error) {
	if eip.HealthCheckPort == port {
		return eip, nil
	}
	s.logger.Info("Update elastic IP health-check port")
	eip.HealthCheckPort = port
	if err := s.cloud.UpdateElasticIP(ctx, eip); err != nil {
		return domain.ElasticIP{}, fmt.Errorf("error while updating the eip: %w", err)
	}
	return eip, nil
}

func (s *elasticIPService) FindElasticIP(ctx context.Context, clusterID uuid.UUID) (domain.ElasticIP, error) {
	eips, err := s.cloud.ListElasticIPs(ctx)
	if err != nil {
		return domain.ElasticIP{}, err
	}
	description := fmt.Sprintf("capi - clusterID - %s", clusterID)
	var match *domain.ElasticIP
	for i := range eips {
		if eips[i].Description != description {
			continue
		}
		if match != nil {
			return domain.ElasticIP{}, fmt.Errorf("multiple elastic IPs described %q", description)
		}
		match = &eips[i]
	}
	if match == nil {
		return domain.ElasticIP{}, domain.ErrElasticIPNotFound
	}
	return *match, nil
}

func (s elasticIPService) DeleteElasticIP(ctx context.Context, id, clusterID uuid.UUID) error {
	eip, err := s.cloud.GetElasticIP(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrElasticIPNotFound) {
			return nil
		}
		return err
	}
	expectedDescription := fmt.Sprintf("capi - clusterID - %s", clusterID)
	if eip.Description != expectedDescription {
		return fmt.Errorf("elastic IP %s is not owned by cluster %s: description is %q", id, clusterID, eip.Description)
	}

	s.logger.Info("Delete elastic IP")
	if err := s.cloud.DeleteElasticIP(ctx, id); err != nil {
		return err
	}

	return nil
}
