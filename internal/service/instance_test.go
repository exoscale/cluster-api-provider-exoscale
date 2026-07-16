package service

import (
	"context"
	"testing"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_instanceService_UpsertInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	machineID := uuid.New()
	instanceID := uuid.New()
	templateID := uuid.New()
	instanceTypeID := uuid.New().String()
	template := domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15 * bytesPerGiB}
	instanceType := domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"}
	spec := domain.InstanceSpec{Name: "machine-0", Template: "ubuntu", InstanceType: "standard-2"}
	instance := domain.Instance{ID: instanceID, Labels: map[string]string{machineUIDLabel: machineID.String()}}

	tests := []struct {
		name       string
		instanceID *uuid.UUID
		cloud      fakeInstanceCloud
		output     domain.Instance
		err        error
	}{
		{
			name:       "reuses instance from status id",
			instanceID: &instanceID,
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, id uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, instanceID, id)
					return instance, nil
				},
			},
			output: instance,
		},
		{
			name: "reuses instance by machine uid label",
			cloud: fakeInstanceCloud{
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return []domain.Instance{instance}, nil
				},
			},
			output: instance,
		},
		{
			name: "creates instance when none exists",
			cloud: fakeInstanceCloud{
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return []domain.Instance{}, nil
				},
				listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
					return []domain.InstanceTemplate{template}, nil
				},
				listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
					return []domain.InstanceType{instanceType}, nil
				},
				createInstance: func(_ context.Context, spec domain.ResolvedInstanceSpec) (uuid.UUID, error) {
					assert.Equal(t, domain.ResolvedInstanceSpec{
						Name:         "machine-0",
						TemplateID:   templateID,
						InstanceType: instanceType,
						DiskSizeGB:   15,
						Labels: map[string]string{
							machineUIDLabel: machineID.String(),
						},
					}, spec)
					return instanceID, nil
				},
				getInstance: func(_ context.Context, id uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, instanceID, id)
					return instance, nil
				},
			},
			output: instance,
		},
		{
			name:       "returns get error",
			instanceID: &instanceID,
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, id uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, instanceID, id)
					return domain.Instance{}, assert.AnError
				},
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			svc := instanceService{cloud: ut.cloud, logger: logr.Discard()}
			output, err := svc.UpsertInstance(ctx, machineID, ut.instanceID, spec)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_instanceService_resolveInstanceSpec(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	instanceTypeID := uuid.New().String()
	template := domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15 * bytesPerGiB}
	instanceType := domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"}
	rootVolumeSizeGB := int64(20)

	t.Run("resolves template name and default disk", func(t *testing.T) {
		t.Parallel()

		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{template}, nil
			},
			listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
				return []domain.InstanceType{instanceType}, nil
			},
		}}

		output, err := svc.resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard-2"})

		assert.NoError(t, err)
		assert.Equal(t, domain.ResolvedInstanceSpec{
			TemplateID:   templateID,
			InstanceType: instanceType,
			DiskSizeGB:   15,
		}, output)
	})

	t.Run("resolves template id and disk override", func(t *testing.T) {
		t.Parallel()

		svc := instanceService{cloud: fakeInstanceCloud{
			getTemplate: func(_ context.Context, id uuid.UUID) (domain.InstanceTemplate, error) {
				assert.Equal(t, templateID, id)
				return template, nil
			},
			listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
				return []domain.InstanceType{instanceType}, nil
			},
		}}

		output, err := svc.resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template:         templateID.String(),
			InstanceType:     "standard-2",
			RootVolumeSizeGB: &rootVolumeSizeGB,
		})

		assert.NoError(t, err)
		assert.Equal(t, int64(20), output.DiskSizeGB)
		assert.Equal(t, templateID, output.TemplateID)
	})

	t.Run("rejects override below template size", func(t *testing.T) {
		t.Parallel()

		tooSmall := int64(10)
		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{template}, nil
			},
			listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
				return []domain.InstanceType{instanceType}, nil
			},
		}}

		_, err := svc.resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template:         "ubuntu",
			InstanceType:     "standard-2",
			RootVolumeSizeGB: &tooSmall,
		})

		assert.ErrorContains(t, err, "rootVolumeSizeGB 10 is smaller than template size 15")
	})
}

func Test_normalizeInstanceType(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"small":      "small",
		"standard-2": "standard.2",
		"memory-4":   "memory.4",
		"compute-8":  "cpu.8",
		"cpu.8":      "cpu.8",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, expected, normalizeInstanceType(input))
		})
	}
}

func Test_instanceService_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name  string
		cloud fakeInstanceCloud
		err   error
	}{
		{
			name: "deletes existing instance",
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, id, gotID)
					return domain.Instance{ID: id}, nil
				},
				deleteInstance: func(_ context.Context, gotID uuid.UUID) error {
					assert.Equal(t, id, gotID)
					return nil
				},
			},
		},
		{
			name: "missing instance is already deleted",
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, id, gotID)
					return domain.Instance{}, domain.ErrInstanceNotFound
				},
			},
		},
		{
			name: "returns get error",
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, id, gotID)
					return domain.Instance{}, assert.AnError
				},
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			svc := instanceService{cloud: ut.cloud, logger: logr.Discard()}
			err := svc.DeleteInstance(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

type fakeInstanceCloud struct {
	listInstances     func(context.Context) ([]domain.Instance, error)
	listInstanceTypes func(context.Context) ([]domain.InstanceType, error)
	getTemplate       func(context.Context, uuid.UUID) (domain.InstanceTemplate, error)
	listTemplates     func(context.Context) ([]domain.InstanceTemplate, error)
	createInstance    func(context.Context, domain.ResolvedInstanceSpec) (uuid.UUID, error)
	getInstance       func(context.Context, uuid.UUID) (domain.Instance, error)
	deleteInstance    func(context.Context, uuid.UUID) error
}

func (f fakeInstanceCloud) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	if f.listInstances == nil {
		panic("unexpected ListInstances")
	}
	return f.listInstances(ctx)
}

func (f fakeInstanceCloud) ListInstanceTypes(ctx context.Context) ([]domain.InstanceType, error) {
	if f.listInstanceTypes == nil {
		panic("unexpected ListInstanceTypes")
	}
	return f.listInstanceTypes(ctx)
}

func (f fakeInstanceCloud) GetTemplate(ctx context.Context, id uuid.UUID) (domain.InstanceTemplate, error) {
	if f.getTemplate == nil {
		panic("unexpected GetTemplate")
	}
	return f.getTemplate(ctx, id)
}

func (f fakeInstanceCloud) ListTemplates(ctx context.Context) ([]domain.InstanceTemplate, error) {
	if f.listTemplates == nil {
		panic("unexpected ListTemplates")
	}
	return f.listTemplates(ctx)
}

func (f fakeInstanceCloud) CreateInstance(ctx context.Context, spec domain.ResolvedInstanceSpec) (uuid.UUID, error) {
	if f.createInstance == nil {
		panic("unexpected CreateInstance")
	}
	return f.createInstance(ctx, spec)
}

func (f fakeInstanceCloud) GetInstance(ctx context.Context, id uuid.UUID) (domain.Instance, error) {
	if f.getInstance == nil {
		panic("unexpected GetInstance")
	}
	return f.getInstance(ctx, id)
}

func (f fakeInstanceCloud) DeleteInstance(ctx context.Context, id uuid.UUID) error {
	if f.deleteInstance == nil {
		panic("unexpected DeleteInstance")
	}
	return f.deleteInstance(ctx, id)
}
