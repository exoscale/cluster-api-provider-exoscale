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
	spec := domain.InstanceSpec{Name: "machine-0"}
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
				createInstance: func(_ context.Context, spec domain.InstanceSpec) (uuid.UUID, error) {
					assert.Equal(t, domain.InstanceSpec{
						Name: "machine-0",
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
	listInstances  func(context.Context) ([]domain.Instance, error)
	createInstance func(context.Context, domain.InstanceSpec) (uuid.UUID, error)
	getInstance    func(context.Context, uuid.UUID) (domain.Instance, error)
	deleteInstance func(context.Context, uuid.UUID) error
}

func (f fakeInstanceCloud) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	if f.listInstances == nil {
		panic("unexpected ListInstances")
	}
	return f.listInstances(ctx)
}

func (f fakeInstanceCloud) CreateInstance(ctx context.Context, spec domain.InstanceSpec) (uuid.UUID, error) {
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
