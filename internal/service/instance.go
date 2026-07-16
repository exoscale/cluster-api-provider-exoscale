package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/infrastructure/exoscale"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// machineUIDLabel tags Exoscale compute instances so the controller can recover
// the instance it created for a given CAPI Machine across reconcile restarts.
const machineUIDLabel = "cluster-api-provider-exoscale/machine-uid"

const bytesPerGiB int64 = 1024 * 1024 * 1024

var _ domain.InstanceService = (*instanceService)(nil)

// instanceService implements domain.InstanceService on top of the domain
// Cloud abstraction, hiding the egoscale SDK behind the higher-level
// UpsertInstance / DeleteInstance operations used by the controller.
type instanceService struct {
	cloud  instanceCloud
	logger logr.Logger
}

// instanceCloud is declared locally so the service can be unit-tested with a
// small fake without pulling in the full egoscale SDK.
type instanceCloud interface {
	ListInstances(ctx context.Context) ([]domain.Instance, error)
	ListInstanceTypes(ctx context.Context) ([]domain.InstanceType, error)
	GetTemplate(ctx context.Context, id uuid.UUID) (domain.InstanceTemplate, error)
	ListTemplates(ctx context.Context) ([]domain.InstanceTemplate, error)
	CreateInstance(ctx context.Context, spec domain.ResolvedInstanceSpec) (uuid.UUID, error)
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
	resolvedSpec, err := s.resolveInstanceSpec(ctx, spec)
	if err != nil {
		return domain.Instance{}, err
	}
	id, err := s.cloud.CreateInstance(ctx, resolvedSpec)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("error while creating instance: %w", err)
	}

	instance, err = s.cloud.GetInstance(ctx, id)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("error while fetching new instance: %w", err)
	}

	return instance, nil
}

func (s *instanceService) resolveInstanceSpec(ctx context.Context, spec domain.InstanceSpec) (domain.ResolvedInstanceSpec, error) {
	template, err := s.resolveTemplate(ctx, spec.Template)
	if err != nil {
		return domain.ResolvedInstanceSpec{}, err
	}
	if template.SizeBytes <= 0 {
		return domain.ResolvedInstanceSpec{}, fmt.Errorf("template %q has invalid size %d", spec.Template, template.SizeBytes)
	}

	instanceType, err := s.resolveInstanceType(ctx, spec.InstanceType)
	if err != nil {
		return domain.ResolvedInstanceSpec{}, err
	}

	templateSizeGB := templateDiskSizeGB(template.SizeBytes)
	diskSize := templateSizeGB
	if spec.RootVolumeSizeGB != nil {
		if *spec.RootVolumeSizeGB < templateSizeGB {
			return domain.ResolvedInstanceSpec{}, fmt.Errorf("rootVolumeSizeGB %d is smaller than template size %d", *spec.RootVolumeSizeGB, templateSizeGB)
		}
		diskSize = *spec.RootVolumeSizeGB
	}

	return domain.ResolvedInstanceSpec{
		Name:             spec.Name,
		TemplateID:       template.ID,
		InstanceType:     instanceType,
		SSHKey:           spec.SSHKey,
		SecurityGroupIDs: spec.SecurityGroupIDs,
		DiskSizeGB:       diskSize,
		UserData:         spec.UserData,
		Labels:           spec.Labels,
	}, nil
}

func (s *instanceService) resolveTemplate(ctx context.Context, templateRef string) (domain.InstanceTemplate, error) {
	if id, err := uuid.Parse(templateRef); err == nil {
		template, err := s.cloud.GetTemplate(ctx, id)
		if err != nil {
			return domain.InstanceTemplate{}, fmt.Errorf("unable to get template %q: %w", templateRef, err)
		}
		return template, nil
	}

	templates, err := s.cloud.ListTemplates(ctx)
	if err != nil {
		return domain.InstanceTemplate{}, fmt.Errorf("unable to list templates: %w", err)
	}
	for _, template := range templates {
		if template.Name == templateRef {
			return template, nil
		}
	}

	return domain.InstanceTemplate{}, fmt.Errorf("unable to find template %q", templateRef)
}

func (s *instanceService) resolveInstanceType(ctx context.Context, instanceType string) (domain.InstanceType, error) {
	instanceTypes, err := s.cloud.ListInstanceTypes(ctx)
	if err != nil {
		return domain.InstanceType{}, fmt.Errorf("unable to list instance types: %w", err)
	}

	normalized := normalizeInstanceType(instanceType)
	for _, candidate := range instanceTypes {
		if candidate.ID == normalized || instanceTypeKey(candidate) == normalized {
			return candidate, nil
		}
	}

	return domain.InstanceType{}, fmt.Errorf("unable to find instance type %q", instanceType)
}

func normalizeInstanceType(instanceType string) string {
	if strings.Contains(instanceType, ".") {
		return instanceType
	}

	for _, family := range []string{"standard", "memory", "startup"} {
		if suffix, ok := strings.CutPrefix(instanceType, family+"-"); ok {
			return family + "." + suffix
		}
	}
	if suffix, ok := strings.CutPrefix(instanceType, "compute-"); ok {
		return "cpu." + suffix
	}
	if instanceType == "compute" {
		return "cpu"
	}

	return instanceType
}

func instanceTypeKey(instanceType domain.InstanceType) string {
	if instanceType.Size == "" {
		return instanceType.Family
	}
	return instanceType.Family + "." + instanceType.Size
}

func templateDiskSizeGB(size int64) int64 {
	return (size + bytesPerGiB - 1) / bytesPerGiB
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
