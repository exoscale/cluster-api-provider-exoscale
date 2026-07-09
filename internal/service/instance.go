package service

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/infrastructure/exoscale"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// machineUIDLabel tags Exoscale compute instances so the controller can recover
// the instance it created for a given CAPI Machine across reconcile restarts.
const machineUIDLabel = "cluster-api-provider-exoscale/machine-uid"

var _ domain.InstanceService = (*instanceService)(nil)

// instanceService implements domain.InstanceService on top of the domain
// Cloud abstraction, hiding the egoscale SDK behind the higher-level
// UpsertInstance / DeleteInstance operations used by the controller.
type instanceService struct {
	cloud  instanceCloud
	logger logr.Logger
}

// instanceCloud is the subset of domain.Cloud that the instance service
// needs. It is declared as its own interface so the service can be unit-tested
// with a local fake without pulling in the full egoscale SDK.
type instanceCloud interface {
	ListInstances(ctx context.Context) ([]domain.Instance, error)
	CreateInstance(ctx context.Context, spec domain.InstanceSpec) (uuid.UUID, error)
	GetInstance(ctx context.Context, id uuid.UUID) (domain.Instance, error)
	DeleteInstance(ctx context.Context, id uuid.UUID) error
}

// NewInstanceService returns an InstanceService bound to the given Exoscale
// zone and credentials. The underlying egoscale client is created lazily by
// the Cloud adapter.
func NewInstanceService(apiKey, apiSecret string, zone egoscale.ZoneName, logger logr.Logger) (domain.InstanceService, error) {
	cloudClient, err := exoscale.NewCloud(apiKey, apiSecret, zone)
	if err != nil {
		return nil, err
	}

	return &instanceService{cloud: cloudClient, logger: logger}, nil
}

func (s *instanceService) UpsertInstance(ctx context.Context, machineID uuid.UUID, instanceID *uuid.UUID, spec domain.InstanceSpec) (domain.Instance, error) {
	spec.Labels = labelsWithMachineID(spec.Labels, machineID)

	if instanceID != nil {
		instance, err := s.cloud.GetInstance(ctx, *instanceID)
		if err == nil {
			return instance, nil
		}
		if !errors.Is(err, domain.ErrInstanceNotFound) {
			return domain.Instance{}, fmt.Errorf("error while fetching instance: %w", err)
		}
		s.logger.Info("Instance not found, will create a new one", "instanceID", instanceID.String())
	}

	instance, err := s.findInstanceByMachineID(ctx, machineID)
	if err == nil {
		s.logger.Info("Found an existing instance by Machine UID, reusing it", "instanceID", instance.ID.String())
		return instance, nil
	}
	if !errors.Is(err, domain.ErrInstanceNotFound) {
		return domain.Instance{}, fmt.Errorf("error while searching for existing instance: %w", err)
	}

	s.logger.Info("Create instance", "machineID", machineID.String())
	id, err := s.cloud.CreateInstance(ctx, spec)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("error while creating instance: %w", err)
	}

	instance, err = s.cloud.GetInstance(ctx, id)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("error while fetching new instance: %w", err)
	}

	return instance, nil
}

func (s *instanceService) DeleteInstance(ctx context.Context, id uuid.UUID) error {
	if _, err := s.cloud.GetInstance(ctx, id); err != nil {
		if errors.Is(err, domain.ErrInstanceNotFound) {
			return nil
		}
		return err
	}

	s.logger.Info("Delete instance", "instanceID", id.String())
	return s.cloud.DeleteInstance(ctx, id)
}

func (s *instanceService) findInstanceByMachineID(ctx context.Context, machineID uuid.UUID) (domain.Instance, error) {
	instances, err := s.cloud.ListInstances(ctx)
	if err != nil {
		return domain.Instance{}, err
	}

	for _, instance := range instances {
		if instance.Labels[machineUIDLabel] == machineID.String() {
			return instance, nil
		}
	}

	return domain.Instance{}, domain.ErrInstanceNotFound
}

func labelsWithMachineID(labels map[string]string, machineID uuid.UUID) map[string]string {
	out := make(map[string]string, len(labels)+1)
	maps.Copy(out, labels)
	out[machineUIDLabel] = machineID.String()
	return out
}
