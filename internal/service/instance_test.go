package service

import (
	"context"
	"testing"
	"time"

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
	elasticIPID := uuid.New()
	instanceTypeID := uuid.New().String()
	template := domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15 * bytesPerGiB}
	instanceType := domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"}
	spec := domain.InstanceSpec{Name: "machine-0", Template: "ubuntu", InstanceType: "standard-2"}
	instance := domain.Instance{ID: instanceID, Labels: map[string]string{machineUIDLabel: machineID.String()}}

	tests := []struct {
		name        string
		instanceID  *uuid.UUID
		elasticIPID *uuid.UUID
		cloud       fakeInstanceCloud
		output      domain.Instance
		err         error
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
			name:        "attaches elastic ip to recovered instance",
			elasticIPID: &elasticIPID,
			cloud: fakeInstanceCloud{
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return []domain.Instance{instance}, nil
				},
				attachInstanceToElasticIP: func(_ context.Context, gotInstanceID, gotElasticIPID uuid.UUID) error {
					assert.Equal(t, instanceID, gotInstanceID)
					assert.Equal(t, elasticIPID, gotElasticIPID)
					return nil
				},
			},
			output: instance,
		},
		{
			name:        "returns elastic ip attachment error",
			instanceID:  &instanceID,
			elasticIPID: &elasticIPID,
			cloud: fakeInstanceCloud{
				getInstance: func(context.Context, uuid.UUID) (domain.Instance, error) {
					return instance, nil
				},
				attachInstanceToElasticIP: func(context.Context, uuid.UUID, uuid.UUID) error {
					return assert.AnError
				},
			},
			err: assert.AnError,
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
			testSpec := spec
			testSpec.ElasticIPID = ut.elasticIPID
			output, err := svc.UpsertInstance(ctx, machineID, ut.instanceID, testSpec)

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

		assert.ErrorContains(t, err, "rootVolumeSizeGB 10 is smaller than minimum size 15")
	})

	t.Run("uses the provider disk minimum for smaller templates", func(t *testing.T) {
		t.Parallel()

		smallTemplate := template
		smallTemplate.SizeBytes = 5 * bytesPerGiB
		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{smallTemplate}, nil
			},
			listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
				return []domain.InstanceType{instanceType}, nil
			},
		}}

		output, err := svc.resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard-2"})

		assert.NoError(t, err)
		assert.Equal(t, minimumDiskSizeGB, output.DiskSizeGB)
	})

	t.Run("selects the newest template with a matching name", func(t *testing.T) {
		t.Parallel()

		older := template
		older.CreatedAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		anotherOlder := older
		anotherOlder.ID = uuid.New()
		newer := template
		newer.ID = uuid.New()
		newer.CreatedAt = older.CreatedAt.Add(time.Hour)
		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{older, anotherOlder, newer}, nil
			},
			listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
				return []domain.InstanceType{instanceType}, nil
			},
		}}

		output, err := svc.resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard-2"})

		assert.NoError(t, err)
		assert.Equal(t, newer.ID, output.TemplateID)
	})

	t.Run("rejects a tie between the newest matching templates", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		first := template
		first.CreatedAt = createdAt
		second := template
		second.ID = uuid.New()
		second.CreatedAt = createdAt
		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{first, second}, nil
			},
		}}

		_, err := svc.resolveTemplate(ctx, "ubuntu")

		assert.ErrorContains(t, err, "multiple newest templates")
	})

	t.Run("rejects explicit disk size below the provider minimum", func(t *testing.T) {
		t.Parallel()

		smallTemplate := template
		smallTemplate.SizeBytes = 5 * bytesPerGiB
		tooSmall := int64(9)
		svc := instanceService{cloud: fakeInstanceCloud{
			listTemplates: func(context.Context) ([]domain.InstanceTemplate, error) {
				return []domain.InstanceTemplate{smallTemplate}, nil
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

		assert.ErrorContains(t, err, "rootVolumeSizeGB 9 is smaller than minimum size 10")
	})
}

func Test_instanceService_resolveInstanceType(t *testing.T) {
	t.Parallel()

	want := domain.InstanceType{ID: uuid.NewString(), Family: "standard", Size: "small"}
	svc := instanceService{cloud: fakeInstanceCloud{
		listInstanceTypes: func(context.Context) ([]domain.InstanceType, error) {
			return []domain.InstanceType{want}, nil
		},
	}}

	got, err := svc.resolveInstanceType(context.Background(), "small")

	assert.NoError(t, err)
	assert.Equal(t, want, got)
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
	machineID := uuid.New()
	id := uuid.New()
	staleID := uuid.New()
	instance := domain.Instance{ID: id, Labels: map[string]string{machineUIDLabel: machineID.String()}}

	tests := []struct {
		name       string
		machineID  *uuid.UUID
		instanceID *uuid.UUID
		cloud      fakeInstanceCloud
		err        error
	}{
		{
			name:       "deletes instance from status id",
			instanceID: &id,
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, id, gotID)
					return instance, nil
				},
				deleteInstance: func(_ context.Context, gotID uuid.UUID) error {
					assert.Equal(t, id, gotID)
					return nil
				},
			},
		},
		{
			name:      "recovers instance from machine uid",
			machineID: &machineID,
			cloud: fakeInstanceCloud{
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return []domain.Instance{instance}, nil
				},
				deleteInstance: func(_ context.Context, gotID uuid.UUID) error {
					assert.Equal(t, id, gotID)
					return nil
				},
			},
		},
		{
			name:       "falls back from stale status id to machine uid",
			machineID:  &machineID,
			instanceID: &staleID,
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, staleID, gotID)
					return domain.Instance{}, domain.ErrInstanceNotFound
				},
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return []domain.Instance{instance}, nil
				},
				deleteInstance: func(_ context.Context, gotID uuid.UUID) error {
					assert.Equal(t, id, gotID)
					return nil
				},
			},
		},
		{
			name:       "delete not found is already deleted",
			instanceID: &id,
			cloud: fakeInstanceCloud{
				getInstance: func(context.Context, uuid.UUID) (domain.Instance, error) {
					return instance, nil
				},
				deleteInstance: func(context.Context, uuid.UUID) error {
					return domain.ErrInstanceNotFound
				},
			},
		},
		{
			name:       "returns get error",
			instanceID: &id,
			cloud: fakeInstanceCloud{
				getInstance: func(_ context.Context, gotID uuid.UUID) (domain.Instance, error) {
					assert.Equal(t, id, gotID)
					return domain.Instance{}, assert.AnError
				},
			},
			err: assert.AnError,
		},
		{
			name:      "missing machine instance is already deleted",
			machineID: &machineID,
			cloud: fakeInstanceCloud{
				listInstances: func(context.Context) ([]domain.Instance, error) {
					return nil, nil
				},
			},
		},
		{name: "no identifiers is already deleted"},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			svc := instanceService{cloud: ut.cloud, logger: logr.Discard()}
			err := svc.DeleteInstance(ctx, ut.machineID, ut.instanceID)

			assert.ErrorIs(t, err, ut.err)
		})
	}

	t.Run("deletes all duplicate machine instances", func(t *testing.T) {
		duplicateID := uuid.New()
		deleted := map[uuid.UUID]bool{}
		svc := instanceService{cloud: fakeInstanceCloud{
			getInstance: func(context.Context, uuid.UUID) (domain.Instance, error) {
				return instance, nil
			},
			listInstances: func(context.Context) ([]domain.Instance, error) {
				return []domain.Instance{instance, {ID: duplicateID, Labels: instance.Labels}}, nil
			},
			deleteInstance: func(_ context.Context, id uuid.UUID) error {
				deleted[id] = true
				return nil
			},
		}}

		err := svc.DeleteInstance(ctx, &machineID, &id)

		assert.NoError(t, err)
		assert.Equal(t, map[uuid.UUID]bool{id: true, duplicateID: true}, deleted)
	})

	t.Run("rejects status instance owned by another machine", func(t *testing.T) {
		ownedID := uuid.New()
		svc := instanceService{cloud: fakeInstanceCloud{
			getInstance: func(context.Context, uuid.UUID) (domain.Instance, error) {
				return domain.Instance{ID: id}, nil
			},
			listInstances: func(context.Context) ([]domain.Instance, error) {
				return []domain.Instance{{ID: ownedID, Labels: instance.Labels}}, nil
			},
		}}

		err := svc.DeleteInstance(ctx, &machineID, &id)

		assert.ErrorContains(t, err, "differs from Machine UID instances")
	})
}

type fakeInstanceCloud struct {
	listInstances             func(context.Context) ([]domain.Instance, error)
	listInstanceTypes         func(context.Context) ([]domain.InstanceType, error)
	getTemplate               func(context.Context, uuid.UUID) (domain.InstanceTemplate, error)
	listTemplates             func(context.Context) ([]domain.InstanceTemplate, error)
	createInstance            func(context.Context, domain.ResolvedInstanceSpec) (uuid.UUID, error)
	getInstance               func(context.Context, uuid.UUID) (domain.Instance, error)
	attachInstanceToElasticIP func(context.Context, uuid.UUID, uuid.UUID) error
	deleteInstance            func(context.Context, uuid.UUID) error
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

func (f fakeInstanceCloud) AttachInstanceToElasticIP(ctx context.Context, instanceID, elasticIPID uuid.UUID) error {
	if f.attachInstanceToElasticIP == nil {
		panic("unexpected AttachInstanceToElasticIP")
	}
	return f.attachInstanceToElasticIP(ctx, instanceID, elasticIPID)
}

func (f fakeInstanceCloud) DeleteInstance(ctx context.Context, id uuid.UUID) error {
	if f.deleteInstance == nil {
		panic("unexpected DeleteInstance")
	}
	return f.deleteInstance(ctx, id)
}
