package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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

const minimumDiskSizeGB int64 = 10

const defaultDiskHeadroomGB int64 = 10

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
	AttachInstanceToElasticIP(ctx context.Context, instanceID, elasticIPID uuid.UUID) error
	AttachInstanceToSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error
	DetachInstanceFromSecurityGroup(ctx context.Context, instanceID, securityGroupID uuid.UUID) error
	DeleteInstance(ctx context.Context, id uuid.UUID) error
}

// NewInstanceService returns an InstanceService bound to the given Exoscale
// zone and credentials. The underlying egoscale client is created lazily by
// the Cloud adapter.
func NewInstanceService(apiKey, apiSecret string, zone egoscale.ZoneName, logger logr.Logger, traceAPI bool) (domain.InstanceService, error) {
	var cloudClient instanceCloud
	var err error
	if traceAPI {
		cloudClient, err = exoscale.NewTracedCloud(apiKey, apiSecret, zone, logger)
	} else {
		cloudClient, err = exoscale.NewCloud(apiKey, apiSecret, zone)
	}
	if err != nil {
		return nil, err
	}

	return &instanceService{cloud: cloudClient, logger: logger}, nil
}

func (s *instanceService) UpsertInstance(ctx context.Context, machineID uuid.UUID, instanceID *uuid.UUID, spec domain.InstanceSpec) (domain.Instance, error) {
	spec.Labels = labelsWithMachineID(spec.Labels, machineID)
	matches, err := s.findInstancesByMachineID(ctx, machineID)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("error while searching for existing instance: %w", err)
	}

	var instance domain.Instance
	if instanceID != nil {
		instance, err = s.cloud.GetInstance(ctx, *instanceID)
		switch {
		case err == nil:
		case errors.Is(err, domain.ErrInstanceNotFound):
			instance = domain.Instance{}
			matches = slices.DeleteFunc(matches, func(instance domain.Instance) bool { return instance.ID == *instanceID })
			s.logger.Info("Status instance not found", "instanceID", instanceID.String())
		default:
			return domain.Instance{}, fmt.Errorf("error while fetching instance: %w", err)
		}
	}

	if instance.ID == uuid.Nil && len(matches) > 0 {
		instance = oldestInstance(matches)
		s.logger.Info("Found an existing instance by Machine UID, reusing it", "instanceID", instance.ID.String())
	}

	if instance.ID == uuid.Nil {
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
	}
	if instance.Labels[machineUIDLabel] != machineID.String() {
		return domain.Instance{}, fmt.Errorf("instance %s is not owned by Machine UID %s", instance.ID, machineID)
	}

	for _, duplicate := range matches {
		if duplicate.ID == instance.ID {
			continue
		}
		s.logger.Info("Delete duplicate instance", "instanceID", duplicate.ID.String(), "machineID", machineID.String())
		if err := s.cloud.DeleteInstance(ctx, duplicate.ID); err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
			return domain.Instance{}, fmt.Errorf("delete duplicate instance: %w", err)
		}
	}

	if err := s.ensureSecurityGroups(ctx, instance, spec.SecurityGroupIDs); err != nil {
		return domain.Instance{}, err
	}
	return s.ensureElasticIP(ctx, instance, spec.ElasticIPID)
}

func (s *instanceService) ensureSecurityGroups(ctx context.Context, instance domain.Instance, desired []uuid.UUID) error {
	current := make(map[uuid.UUID]struct{}, len(instance.SecurityGroupIDs))
	for _, id := range instance.SecurityGroupIDs {
		current[id] = struct{}{}
	}
	desiredSet := make(map[uuid.UUID]struct{}, len(desired))
	for _, id := range uniqueUUIDs(desired) {
		desiredSet[id] = struct{}{}
		if _, ok := current[id]; ok {
			continue
		}
		if err := s.cloud.AttachInstanceToSecurityGroup(ctx, instance.ID, id); err != nil {
			return fmt.Errorf("attach instance to security group: %w", err)
		}
	}
	for id := range current {
		if _, ok := desiredSet[id]; ok {
			continue
		}
		if err := s.cloud.DetachInstanceFromSecurityGroup(ctx, instance.ID, id); err != nil {
			return fmt.Errorf("detach instance from security group: %w", err)
		}
	}
	return nil
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	unique := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func (s *instanceService) ensureElasticIP(ctx context.Context, instance domain.Instance, elasticIPID *uuid.UUID) (domain.Instance, error) {
	if elasticIPID == nil {
		return instance, nil
	}
	if err := s.cloud.AttachInstanceToElasticIP(ctx, instance.ID, *elasticIPID); err != nil {
		return domain.Instance{}, fmt.Errorf("attach instance to elastic IP: %w", err)
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
	minimumSizeGB := max(templateSizeGB, minimumDiskSizeGB)
	diskSize := templateSizeGB + defaultDiskHeadroomGB
	if spec.RootVolumeSizeGB != nil {
		if *spec.RootVolumeSizeGB < minimumSizeGB {
			return domain.ResolvedInstanceSpec{}, fmt.Errorf("rootVolumeSizeGB %d is smaller than minimum size %d", *spec.RootVolumeSizeGB, minimumSizeGB)
		}
		diskSize = *spec.RootVolumeSizeGB
	}

	return domain.ResolvedInstanceSpec{
		Name:             spec.Name,
		TemplateID:       template.ID,
		InstanceType:     instanceType,
		SSHKey:           spec.SSHKey,
		SecurityGroupIDs: uniqueUUIDs(spec.SecurityGroupIDs),
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
	var latest *domain.InstanceTemplate
	ambiguousLatest := false
	for i := range templates {
		template := &templates[i]
		if template.Name != templateRef {
			continue
		}
		if latest == nil || template.CreatedAt.After(latest.CreatedAt) {
			latest = template
			ambiguousLatest = false
			continue
		}
		if template.CreatedAt.Equal(latest.CreatedAt) {
			ambiguousLatest = true
		}
	}
	if latest != nil {
		if ambiguousLatest {
			return domain.InstanceTemplate{}, fmt.Errorf("multiple newest templates named %q have the same creation time", templateRef)
		}
		return *latest, nil
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
		if candidate.ID == normalized || instanceTypeKey(candidate) == normalized ||
			(!strings.Contains(normalized, ".") && candidate.Family == "standard" && candidate.Size == normalized) {
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

func (s *instanceService) DeleteInstance(ctx context.Context, machineID, instanceID *uuid.UUID) error {
	var statusInstance domain.Instance
	if instanceID != nil {
		var err error
		statusInstance, err = s.cloud.GetInstance(ctx, *instanceID)
		if err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
			return err
		}
		if err == nil {
			if machineID == nil {
				return fmt.Errorf("cannot verify status instance %s without a Machine UID", statusInstance.ID)
			}
			if statusInstance.Labels[machineUIDLabel] != machineID.String() {
				return fmt.Errorf("status instance %s is not owned by Machine UID %s", statusInstance.ID, machineID)
			}
		}
	}

	var instances []domain.Instance
	if machineID != nil {
		byMachineID, err := s.findInstancesByMachineID(ctx, *machineID)
		if err != nil {
			return err
		}
		instances = byMachineID
		if statusInstance.ID != uuid.Nil && !slices.ContainsFunc(instances, func(instance domain.Instance) bool { return instance.ID == statusInstance.ID }) {
			instances = append(instances, statusInstance)
		}
	}
	if len(instances) == 0 {
		return nil
	}

	for _, instance := range instances {
		s.logger.Info("Delete instance", "instanceID", instance.ID.String())
		if err := s.cloud.DeleteInstance(ctx, instance.ID); err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
			return err
		}
	}
	return nil
}

func (s *instanceService) findInstancesByMachineID(ctx context.Context, machineID uuid.UUID) ([]domain.Instance, error) {
	instances, err := s.cloud.ListInstances(ctx)
	if err != nil {
		return nil, err
	}

	var matches []domain.Instance
	for _, instance := range instances {
		if instance.Labels[machineUIDLabel] == machineID.String() {
			matches = append(matches, instance)
		}
	}
	return matches, nil
}

func oldestInstance(instances []domain.Instance) domain.Instance {
	oldest := instances[0]
	for _, instance := range instances[1:] {
		if instance.CreatedAt < oldest.CreatedAt || (instance.CreatedAt == oldest.CreatedAt && instance.ID.String() < oldest.ID.String()) {
			oldest = instance
		}
	}
	return oldest
}

func labelsWithMachineID(labels map[string]string, machineID uuid.UUID) map[string]string {
	out := make(map[string]string, len(labels)+1)
	maps.Copy(out, labels)
	out[machineUIDLabel] = machineID.String()
	return out
}
