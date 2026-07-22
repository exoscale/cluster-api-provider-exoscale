package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var errNotOwned = errors.New("not owned")

func Test_instanceService_UpsertInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	machineID := domain.MachineID(uuid.NewString())
	clusterID := uuid.NewString()
	query := domain.MachineUIDKey + "=" + machineID.String()
	instanceID := uuid.New()
	templateID := uuid.New()
	elasticIPID := uuid.New()
	instanceTypeID := uuid.NewString()
	template := domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15 * bytesPerGiB}
	instanceType := domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "small"}
	spec := domain.InstanceSpec{
		Name: "machine-0", Template: "ubuntu", InstanceType: "standard.small",
		Labels: map[string]string{domain.ClusterIDKey: clusterID},
	}
	labels := map[string]string{domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: clusterID}
	instance := domain.Instance{ID: instanceID, Labels: labels}

	tests := []struct {
		name        string
		instanceID  *uuid.UUID
		elasticIPID *uuid.UUID
		missingID   bool
		cloud       func(*mocks.InstanceCloud)
		output      domain.Instance
		err         error
		errContains string
	}{
		{
			name:       "reuses instance from status ID",
			instanceID: &instanceID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(instance, nil)
			},
			output: instance,
		},
		{
			name: "recovers only instance with both ownership labels",
			cloud: func(m *mocks.InstanceCloud) {
				foreign := instance
				foreign.ID = uuid.New()
				foreign.Labels = map[string]string{domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: uuid.NewString()}
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{foreign, instance}, nil)
			},
			output: instance,
		},
		{
			name:        "attaches elastic IP to recovered instance",
			elasticIPID: &elasticIPID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().AttachInstanceToElasticIP(ctx, instanceID, elasticIPID).Return(nil)
			},
			output: instance,
		},
		{
			name:        "returns elastic IP attachment error",
			instanceID:  &instanceID,
			elasticIPID: &elasticIPID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(instance, nil)
				m.EXPECT().AttachInstanceToElasticIP(ctx, instanceID, elasticIPID).Return(assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "creates instance when none exists",
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return(nil, nil)
				m.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{template}, nil)
				m.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)
				m.EXPECT().CreateInstance(ctx, domain.ResolvedInstanceSpec{
					Name:         "machine-0",
					TemplateID:   templateID,
					InstanceType: instanceType,
					DiskSizeGiB:  25,
					Labels:       labels,
				}).Return(instanceID, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(instance, nil)
			},
			output: instance,
		},
		{
			name:       "returns get error",
			instanceID: &instanceID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return(nil, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(domain.Instance{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name:       "rejects status instance owned by another machine",
			instanceID: &instanceID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return(nil, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(domain.Instance{ID: instanceID, Labels: map[string]string{
					domain.MachineUIDKey: uuid.NewString(), domain.ClusterIDKey: clusterID,
				}}, nil)
			},
			errContains: "is not owned by Machine UID",
		},
		{
			name:       "rejects status instance owned by another cluster",
			instanceID: &instanceID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return(nil, nil)
				m.EXPECT().GetInstance(ctx, instanceID).Return(domain.Instance{ID: instanceID, Labels: map[string]string{
					domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: uuid.NewString(),
				}}, nil)
			},
			errContains: "is not owned by cluster",
		},
		{
			name:        "requires cluster ownership label",
			missingID:   true,
			errContains: "cluster ID label is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cloud := mocks.NewInstanceCloud(t)
			if tc.cloud != nil {
				tc.cloud(cloud)
			}
			testSpec := spec
			testSpec.ElasticIPID = tc.elasticIPID
			if tc.missingID {
				testSpec.Labels = nil
			}

			output, err := (&instanceService{cloud: cloud, logger: logr.Discard()}).UpsertInstance(ctx, machineID, tc.instanceID, testSpec)

			if tc.errContains != "" {
				assert.ErrorContains(t, err, tc.errContains)
			} else {
				assert.ErrorIs(t, err, tc.err)
			}
			assert.Equal(t, tc.output, output)
		})
	}

	t.Run("keeps oldest duplicate and deletes the rest", func(t *testing.T) {
		oldest := domain.Instance{ID: uuid.New(), CreatedAt: "2026-07-15T10:00:00Z", Labels: labels}
		newest := domain.Instance{ID: uuid.New(), CreatedAt: "2026-07-15T11:00:00Z", Labels: labels}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{newest, oldest}, nil)
		cloud.EXPECT().DeleteInstance(ctx, newest.ID).Return(nil)

		output, err := (&instanceService{cloud: cloud, logger: logr.Discard()}).UpsertInstance(ctx, machineID, nil, spec)

		assert.NoError(t, err)
		assert.Equal(t, oldest, output)
	})

	t.Run("keeps owned status instance and deletes duplicates", func(t *testing.T) {
		oldest := domain.Instance{ID: uuid.New(), CreatedAt: "2026-07-15T10:00:00Z", Labels: labels}
		statusInstance := domain.Instance{ID: uuid.New(), CreatedAt: "2026-07-15T11:00:00Z", Labels: labels}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{oldest, statusInstance}, nil)
		cloud.EXPECT().GetInstance(ctx, statusInstance.ID).Return(statusInstance, nil)
		cloud.EXPECT().DeleteInstance(ctx, oldest.ID).Return(nil)

		output, err := (&instanceService{cloud: cloud, logger: logr.Discard()}).UpsertInstance(ctx, machineID, &statusInstance.ID, spec)

		assert.NoError(t, err)
		assert.Equal(t, statusInstance, output)
	})

	t.Run("ignores stale listed status instance", func(t *testing.T) {
		staleID := uuid.New()
		healthy := domain.Instance{ID: uuid.New(), CreatedAt: "2026-07-15T11:00:00Z", Labels: labels}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{
			{ID: staleID, CreatedAt: "2026-07-15T10:00:00Z", Labels: labels}, healthy,
		}, nil)
		cloud.EXPECT().GetInstance(ctx, staleID).Return(domain.Instance{}, domain.ErrInstanceNotFound)

		output, err := (&instanceService{cloud: cloud, logger: logr.Discard()}).UpsertInstance(ctx, machineID, &staleID, spec)

		assert.NoError(t, err)
		assert.Equal(t, healthy, output)
	})

	t.Run("converges security groups", func(t *testing.T) {
		keepID, attachID, detachID := uuid.New(), uuid.New(), uuid.New()
		withGroups := instance
		withGroups.SecurityGroupIDs = []uuid.UUID{keepID, detachID}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{withGroups}, nil)
		cloud.EXPECT().AttachInstanceToSecurityGroup(ctx, instanceID, attachID).Return(nil)
		cloud.EXPECT().DetachInstanceFromSecurityGroup(ctx, instanceID, detachID).Return(nil)
		testSpec := spec
		testSpec.SecurityGroupIDs = []uuid.UUID{keepID, attachID, attachID}

		_, err := (&instanceService{cloud: cloud, logger: logr.Discard()}).UpsertInstance(ctx, machineID, nil, testSpec)

		assert.NoError(t, err)
	})
}

func Test_instanceService_resolveInstanceSpec(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	securityGroupID := uuid.New()
	template := domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15 * bytesPerGiB}
	instanceType := domain.InstanceType{ID: uuid.NewString(), Family: "standard", Size: "small"}

	t.Run("resolves template name and default disk", func(t *testing.T) {
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{template}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		output, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template: "ubuntu", InstanceType: "standard.small", SecurityGroupIDs: []uuid.UUID{securityGroupID, securityGroupID},
		})

		assert.NoError(t, err)
		assert.Equal(t, domain.ResolvedInstanceSpec{
			TemplateID: templateID, InstanceType: instanceType,
			SecurityGroupIDs: []uuid.UUID{securityGroupID}, DiskSizeGiB: 25,
		}, output)
	})

	t.Run("resolves template ID and disk override", func(t *testing.T) {
		rootVolumeSizeGiB := int64(20)
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().GetTemplate(ctx, templateID).Return(template, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		output, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template: templateID.String(), InstanceType: "standard.small", RootVolumeSizeGiB: &rootVolumeSizeGiB,
		})

		assert.NoError(t, err)
		assert.Equal(t, int64(20), output.DiskSizeGiB)
		assert.Equal(t, templateID, output.TemplateID)
	})

	t.Run("rejects override below template size", func(t *testing.T) {
		tooSmall := int64(10)
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{template}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		_, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template: "ubuntu", InstanceType: "standard.small", RootVolumeSizeGiB: &tooSmall,
		})

		assert.ErrorContains(t, err, "rootVolumeSizeGiB 10 is smaller than minimum size 15")
	})

	t.Run("adds default headroom to smaller templates", func(t *testing.T) {
		smallTemplate := template
		smallTemplate.SizeBytes = 5 * bytesPerGiB
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{smallTemplate}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		output, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard.small"})

		assert.NoError(t, err)
		assert.Equal(t, int64(15), output.DiskSizeGiB)
	})

	t.Run("rounds template bytes up to GiB", func(t *testing.T) {
		fractionalTemplate := template
		fractionalTemplate.SizeBytes++
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{fractionalTemplate}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		output, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard.small"})

		assert.NoError(t, err)
		assert.Equal(t, int64(26), output.DiskSizeGiB)
	})

	t.Run("selects newest matching template", func(t *testing.T) {
		older := template
		older.CreatedAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		newer := template
		newer.ID = uuid.New()
		newer.CreatedAt = older.CreatedAt.Add(time.Hour)
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{older, newer}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		output, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{Template: "ubuntu", InstanceType: "standard.small"})

		assert.NoError(t, err)
		assert.Equal(t, newer.ID, output.TemplateID)
	})

	t.Run("rejects tie between newest matching templates", func(t *testing.T) {
		createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		first := template
		first.CreatedAt = createdAt
		second := first
		second.ID = uuid.New()
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{first, second}, nil)

		_, err := (&instanceService{cloud: cloud}).resolveTemplate(ctx, "ubuntu")

		assert.ErrorContains(t, err, "multiple newest templates")
	})

	t.Run("rejects explicit disk size below provider minimum", func(t *testing.T) {
		smallTemplate := template
		smallTemplate.SizeBytes = 5 * bytesPerGiB
		tooSmall := int64(9)
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListTemplates(ctx).Return([]domain.InstanceTemplate{smallTemplate}, nil)
		cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{instanceType}, nil)

		_, err := (&instanceService{cloud: cloud}).resolveInstanceSpec(ctx, domain.InstanceSpec{
			Template: "ubuntu", InstanceType: "standard.small", RootVolumeSizeGiB: &tooSmall,
		})

		assert.ErrorContains(t, err, "rootVolumeSizeGiB 9 is smaller than minimum size 10")
	})
}

func Test_instanceService_resolveInstanceType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	standard := domain.InstanceType{ID: uuid.NewString(), Family: "standard", Size: "small"}
	memory := domain.InstanceType{ID: uuid.NewString(), Family: "memory", Size: "large"}
	cpu := domain.InstanceType{ID: uuid.NewString(), Family: "cpu", Size: "extra-large"}
	cloud := mocks.NewInstanceCloud(t)
	cloud.EXPECT().ListInstanceTypes(ctx).Return([]domain.InstanceType{standard, memory, cpu}, nil).Times(6)
	svc := instanceService{cloud: cloud}

	tests := map[string]domain.InstanceType{
		"small": standard, "standard.small": standard, "memory.large": memory,
		"cpu.extra-large": cpu, memory.ID: memory,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := svc.resolveInstanceType(ctx, input)
			assert.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	_, err := svc.resolveInstanceType(ctx, "memory.unknown")
	assert.ErrorContains(t, err, `unable to find instance type "memory.unknown"`)
}

func Test_instanceService_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	machineID := domain.MachineID(uuid.NewString())
	clusterID := uuid.New()
	query := domain.MachineUIDKey + "=" + machineID.String()
	id := uuid.New()
	staleID := uuid.New()
	labels := map[string]string{domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: clusterID.String()}
	instance := domain.Instance{ID: id, Labels: labels}

	tests := []struct {
		name        string
		machineID   *domain.MachineID
		clusterID   uuid.UUID
		instanceID  *uuid.UUID
		cloud       func(*mocks.InstanceCloud)
		err         error
		errContains string
	}{
		{
			name: "deletes instance from status ID", machineID: &machineID, clusterID: clusterID, instanceID: &id,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().GetInstance(ctx, id).Return(instance, nil)
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().DeleteInstance(ctx, id).Return(nil)
			},
		},
		{
			name: "recovers instance from ownership labels", machineID: &machineID, clusterID: clusterID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().DeleteInstance(ctx, id).Return(nil)
			},
		},
		{
			name: "falls back from stale status ID", machineID: &machineID, clusterID: clusterID, instanceID: &staleID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().GetInstance(ctx, staleID).Return(domain.Instance{}, domain.ErrInstanceNotFound)
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().DeleteInstance(ctx, id).Return(nil)
			},
		},
		{
			name: "delete not found is already deleted", machineID: &machineID, clusterID: clusterID, instanceID: &id,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().GetInstance(ctx, id).Return(instance, nil)
				m.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance}, nil)
				m.EXPECT().DeleteInstance(ctx, id).Return(domain.ErrInstanceNotFound)
			},
		},
		{
			name: "returns get error", machineID: &machineID, clusterID: clusterID, instanceID: &id,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().GetInstance(ctx, id).Return(domain.Instance{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "missing machine instance is already deleted", machineID: &machineID, clusterID: clusterID,
			cloud: func(m *mocks.InstanceCloud) {
				m.EXPECT().ListInstances(ctx, query).Return(nil, nil)
			},
		},
		{name: "no identifiers is already deleted"},
		{
			name: "rejects status ID without Machine UID", clusterID: clusterID, instanceID: &id,
			errContains: "without a Machine UID",
		},
		{
			name: "requires cluster ID", machineID: &machineID,
			errContains: "without a cluster ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cloud := mocks.NewInstanceCloud(t)
			if tc.cloud != nil {
				tc.cloud(cloud)
			}

			err := (&instanceService{cloud: cloud, logger: logr.Discard()}).DeleteInstance(ctx, tc.machineID, tc.clusterID, tc.instanceID)

			if tc.errContains != "" {
				assert.ErrorContains(t, err, tc.errContains)
			} else {
				assert.ErrorIs(t, err, tc.err)
			}
		})
	}

	t.Run("filters cross-cluster instances before deletion", func(t *testing.T) {
		foreign := domain.Instance{ID: uuid.New(), Labels: map[string]string{
			domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: uuid.NewString(),
		}}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{foreign, instance}, nil)
		cloud.EXPECT().DeleteInstance(ctx, id).Return(nil)

		err := (&instanceService{cloud: cloud}).DeleteInstance(ctx, &machineID, clusterID, nil)

		assert.NoError(t, err)
	})

	t.Run("deletes all duplicate owned instances", func(t *testing.T) {
		duplicate := domain.Instance{ID: uuid.New(), Labels: labels}
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().GetInstance(ctx, id).Return(instance, nil)
		cloud.EXPECT().ListInstances(ctx, query).Return([]domain.Instance{instance, duplicate}, nil)
		cloud.EXPECT().DeleteInstance(ctx, id).Return(nil)
		cloud.EXPECT().DeleteInstance(ctx, duplicate.ID).Return(nil)

		err := (&instanceService{cloud: cloud}).DeleteInstance(ctx, &machineID, clusterID, &id)

		assert.NoError(t, err)
	})

	t.Run("rejects status instance owned by another machine", func(t *testing.T) {
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().GetInstance(ctx, id).Return(domain.Instance{ID: id, Labels: map[string]string{
			domain.MachineUIDKey: uuid.NewString(), domain.ClusterIDKey: clusterID.String(),
		}}, nil)

		err := (&instanceService{cloud: cloud}).DeleteInstance(ctx, &machineID, clusterID, &id)

		assert.ErrorContains(t, err, "is not owned by Machine UID")
	})

	t.Run("rejects status instance owned by another cluster", func(t *testing.T) {
		cloud := mocks.NewInstanceCloud(t)
		cloud.EXPECT().GetInstance(ctx, id).Return(domain.Instance{ID: id, Labels: map[string]string{
			domain.MachineUIDKey: machineID.String(), domain.ClusterIDKey: uuid.NewString(),
		}}, nil)

		err := (&instanceService{cloud: cloud}).DeleteInstance(ctx, &machineID, clusterID, &id)

		assert.ErrorContains(t, err, "is not owned by cluster")
	})
}
