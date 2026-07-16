package exoscale

import (
	"context"
	"encoding/base64"
	"net"
	"testing"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/stretchr/testify/assert"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/google/uuid"
)

func Test_cloud_CreateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	healthCheckPort := 12345
	description := "description"
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
				).Return(&egoscale.Operation{}, nil)
			},
			output: id,
		},
		{
			name: "create eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.CreateElasticIP(ctx, int32(healthCheckPort), description)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_GetElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.ElasticIP
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(
						&egoscale.ElasticIP{
							Description: "description",
							IP:          "1.2.3.4",
							Healthcheck: &egoscale.ElasticIPHealthcheck{
								Port: 12345,
							},
						}, nil)
			},
			output: domain.ElasticIP{
				ID:              id,
				IP:              "1.2.3.4",
				HealthCheckPort: 12345,
				Description:     "description",
			},
		},
		{
			name: "get eip returned an error - not fond",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.ElasticIP{}, egoscale.ErrNotFound)
			},
			err: domain.ErrElasticIPNotFound,
		},
		{
			name: "get eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.ElasticIP{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.GetElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_ListElasticIPs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id1 := uuid.New()
	id2 := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.ElasticIP
		err       error
	}{
		{
			name: "nominal - empty list",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).
					Return(&egoscale.ListElasticIPSResponse{ElasticIPS: []egoscale.ElasticIP{}}, nil)
			},
			output: []domain.ElasticIP{},
		},
		{
			name: "nominal - maps fields correctly",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).
					Return(&egoscale.ListElasticIPSResponse{
						ElasticIPS: []egoscale.ElasticIP{
							{
								ID:          egoscale.UUID(id1.String()),
								IP:          "1.2.3.4",
								Description: "capi - clusterID - abc",
								Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 6443},
							},
							{
								ID:          egoscale.UUID(id2.String()),
								IP:          "5.6.7.8",
								Description: "other",
								Healthcheck: &egoscale.ElasticIPHealthcheck{Port: 12345},
							},
						},
					}, nil)
			},
			output: []domain.ElasticIP{
				{ID: id1, IP: "1.2.3.4", Description: "capi - clusterID - abc", HealthCheckPort: 6443},
				{ID: id2, IP: "5.6.7.8", Description: "other", HealthCheckPort: 12345},
			},
		},
		{
			name: "list eips returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.ListElasticIPs(ctx)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_CreateInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	securityGroupID := uuid.New()
	rootVolumeSizeGB := int64(20)
	template := egoscale.Template{ID: egoscale.UUID(templateID.String()), Size: 15 * bytesPerGiB}
	instanceTypeID := egoscale.UUID(uuid.New().String())
	instanceType := egoscale.InstanceType{
		ID:     instanceTypeID,
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	spec := domain.InstanceSpec{
		Name:             "machine-0",
		TemplateID:       templateID,
		InstanceType:     "standard-2",
		SSHKey:           "ssh-key",
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
		RootVolumeSizeGB: &rootVolumeSizeGB,
		UserData:         "#cloud-config",
		Labels:           map[string]string{"machine": "uid"},
	}

	createOp := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
	exoClient := &instanceExoscaleClientFake{
		ExoscaleClient: mocks.NewExoscaleClient(t),
		listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
			return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
		},
		getTemplate: func(_ context.Context, id egoscale.UUID) (*egoscale.Template, error) {
			assert.Equal(t, egoscale.UUID(templateID.String()), id)
			return &template, nil
		},
		createInstance: func(_ context.Context, req egoscale.CreateInstanceRequest) (*egoscale.Operation, error) {
			assert.Equal(t, egoscale.CreateInstanceRequest{
				DiskSize:           rootVolumeSizeGB,
				InstanceType:       &instanceType,
				Labels:             egoscale.Labels{"machine": "uid"},
				Name:               "machine-0",
				PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
				SecurityGroups:     []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
				SSHKey:             &egoscale.SSHKey{Name: "ssh-key"},
				Template:           &egoscale.Template{ID: template.ID},
				UserData:           base64.StdEncoding.EncodeToString([]byte("#cloud-config")),
			}, req)
			return createOp, nil
		},
	}
	exoClient.EXPECT().Wait(ctx, createOp).
		Return(&egoscale.Operation{}, nil)

	client := cloud{exoClient: exoClient, instanceClient: exoClient}

	output, err := client.CreateInstance(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, instanceID, output)
}

func Test_cloud_ListInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	t.Run("maps instances", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstances: func(context.Context, ...egoscale.ListInstancesOpt) (*egoscale.ListInstancesResponse, error) {
				return &egoscale.ListInstancesResponse{Instances: []egoscale.ListInstancesResponseInstances{{
					ID:        egoscale.UUID(id.String()),
					Name:      "machine-0",
					State:     egoscale.InstanceStateRunning,
					PublicIP:  net.ParseIP("1.2.3.4"),
					CreatedAT: createdAt,
					Labels:    egoscale.Labels{"machine": "uid"},
				}}}, nil
			},
		}

		client := cloud{instanceClient: exoClient}

		output, err := client.ListInstances(ctx)

		assert.NoError(t, err)
		assert.Equal(t, []domain.Instance{{
			ID:        id,
			Name:      "machine-0",
			State:     "running",
			PublicIP:  "1.2.3.4",
			CreatedAt: createdAt.Format(time.RFC3339),
			Labels:    map[string]string{"machine": "uid"},
		}}, output)
	})

	t.Run("returns list error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstances: func(context.Context, ...egoscale.ListInstancesOpt) (*egoscale.ListInstancesResponse, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{instanceClient: exoClient}

		output, err := client.ListInstances(ctx)

		assert.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, output)
	})
}

func Test_cloud_GetInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	t.Run("maps instance", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			getInstance: func(_ context.Context, gotID egoscale.UUID) (*egoscale.Instance, error) {
				assert.Equal(t, egoscale.UUID(id.String()), gotID)
				return &egoscale.Instance{
					Name:      "machine-0",
					State:     egoscale.InstanceStateRunning,
					PublicIP:  net.ParseIP("1.2.3.4"),
					CreatedAT: createdAt,
					Labels:    egoscale.Labels{"machine": "uid"},
				}, nil
			},
		}

		client := cloud{instanceClient: exoClient}

		output, err := client.GetInstance(ctx, id)

		assert.NoError(t, err)
		assert.Equal(t, domain.Instance{
			ID:        id,
			Name:      "machine-0",
			State:     "running",
			PublicIP:  "1.2.3.4",
			CreatedAt: createdAt.Format(time.RFC3339),
			Labels:    map[string]string{"machine": "uid"},
		}, output)
	})

	t.Run("maps not found", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			getInstance: func(context.Context, egoscale.UUID) (*egoscale.Instance, error) {
				return nil, egoscale.ErrNotFound
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.GetInstance(ctx, id)

		assert.ErrorIs(t, err, domain.ErrInstanceNotFound)
	})

	t.Run("returns get error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			getInstance: func(context.Context, egoscale.UUID) (*egoscale.Instance, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.GetInstance(ctx, id)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

func Test_cloud_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}}

	t.Run("deletes and waits", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			deleteInstance: func(_ context.Context, gotID egoscale.UUID) (*egoscale.Operation, error) {
				assert.Equal(t, egoscale.UUID(id.String()), gotID)
				return op, nil
			},
		}
		exoClient.EXPECT().Wait(ctx, op).Return(&egoscale.Operation{}, nil)

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		assert.NoError(t, client.DeleteInstance(ctx, id))
	})

	t.Run("returns delete error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			deleteInstance: func(context.Context, egoscale.UUID) (*egoscale.Operation, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		assert.ErrorIs(t, client.DeleteInstance(ctx, id), assert.AnError)
	})

	t.Run("returns wait error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			deleteInstance: func(context.Context, egoscale.UUID) (*egoscale.Operation, error) {
				return op, nil
			},
		}
		exoClient.EXPECT().Wait(ctx, op).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		assert.ErrorIs(t, client.DeleteInstance(ctx, id), assert.AnError)
	})
}

func Test_cloud_CreateInstance_errors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	instanceType := egoscale.InstanceType{
		ID:     egoscale.UUID(uuid.New().String()),
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	template := egoscale.Template{ID: egoscale.UUID(templateID.String()), Size: 10 * bytesPerGiB}
	spec := domain.InstanceSpec{TemplateID: templateID, InstanceType: "standard-2"}

	t.Run("returns list instance types error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("returns instance type lookup error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{}, nil
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorContains(t, err, "unable to find instance type")
	})

	t.Run("returns get template error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(context.Context, egoscale.UUID) (*egoscale.Template, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("rejects invalid template size", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(context.Context, egoscale.UUID) (*egoscale.Template, error) {
				return &egoscale.Template{ID: egoscale.UUID(templateID.String())}, nil
			},
		}

		client := cloud{instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorContains(t, err, "invalid size")
	})

	t.Run("returns create error", func(t *testing.T) {
		t.Parallel()

		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(context.Context, egoscale.UUID) (*egoscale.Template, error) {
				return &template, nil
			},
			createInstance: func(context.Context, egoscale.CreateInstanceRequest) (*egoscale.Operation, error) {
				return nil, assert.AnError
			},
		}

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("returns wait error", func(t *testing.T) {
		t.Parallel()

		op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(context.Context, egoscale.UUID) (*egoscale.Template, error) {
				return &template, nil
			},
			createInstance: func(context.Context, egoscale.CreateInstanceRequest) (*egoscale.Operation, error) {
				return op, nil
			},
		}
		exoClient.EXPECT().Wait(ctx, op).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

func Test_cloud_CreateInstance_diskSize(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	instanceType := egoscale.InstanceType{
		ID:     egoscale.UUID(uuid.New().String()),
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	template := egoscale.Template{ID: egoscale.UUID(templateID.String()), Size: 30 * bytesPerGiB}

	t.Run("defaults to template size", func(t *testing.T) {
		t.Parallel()

		createOp := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(_ context.Context, id egoscale.UUID) (*egoscale.Template, error) {
				assert.Equal(t, egoscale.UUID(templateID.String()), id)
				return &template, nil
			},
			createInstance: func(_ context.Context, req egoscale.CreateInstanceRequest) (*egoscale.Operation, error) {
				assert.Equal(t, int64(30), req.DiskSize)
				assert.Nil(t, req.SSHKey)
				assert.Equal(t, &egoscale.Template{ID: template.ID}, req.Template)
				return createOp, nil
			},
		}
		exoClient.EXPECT().Wait(ctx, createOp).Return(&egoscale.Operation{}, nil)

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		output, err := client.CreateInstance(ctx, domain.InstanceSpec{TemplateID: templateID, InstanceType: "standard-2"})

		assert.NoError(t, err)
		assert.Equal(t, instanceID, output)
	})

	t.Run("rejects override below template size", func(t *testing.T) {
		t.Parallel()

		rootVolumeSizeGB := int64(20)
		exoClient := &instanceExoscaleClientFake{
			ExoscaleClient: mocks.NewExoscaleClient(t),
			listInstanceTypes: func(context.Context) (*egoscale.ListInstanceTypesResponse, error) {
				return &egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{instanceType}}, nil
			},
			getTemplate: func(_ context.Context, id egoscale.UUID) (*egoscale.Template, error) {
				assert.Equal(t, egoscale.UUID(templateID.String()), id)
				return &template, nil
			},
		}

		client := cloud{exoClient: exoClient, instanceClient: exoClient}

		output, err := client.CreateInstance(ctx, domain.InstanceSpec{
			TemplateID:       templateID,
			InstanceType:     "standard-2",
			RootVolumeSizeGB: &rootVolumeSizeGB,
		})

		assert.ErrorContains(t, err, "rootVolumeSizeGB 20 is smaller than template size 30")
		assert.Equal(t, uuid.Nil, output)
	})
}

type instanceExoscaleClientFake struct {
	*mocks.ExoscaleClient
	listInstances     func(context.Context, ...egoscale.ListInstancesOpt) (*egoscale.ListInstancesResponse, error)
	createInstance    func(context.Context, egoscale.CreateInstanceRequest) (*egoscale.Operation, error)
	getInstance       func(context.Context, egoscale.UUID) (*egoscale.Instance, error)
	deleteInstance    func(context.Context, egoscale.UUID) (*egoscale.Operation, error)
	listInstanceTypes func(context.Context) (*egoscale.ListInstanceTypesResponse, error)
	getTemplate       func(context.Context, egoscale.UUID) (*egoscale.Template, error)
}

func (f *instanceExoscaleClientFake) ListInstances(ctx context.Context, opts ...egoscale.ListInstancesOpt) (*egoscale.ListInstancesResponse, error) {
	if f.listInstances == nil {
		panic("unexpected ListInstances")
	}
	return f.listInstances(ctx, opts...)
}

func (f *instanceExoscaleClientFake) CreateInstance(ctx context.Context, req egoscale.CreateInstanceRequest) (*egoscale.Operation, error) {
	if f.createInstance == nil {
		panic("unexpected CreateInstance")
	}
	return f.createInstance(ctx, req)
}

func (f *instanceExoscaleClientFake) GetInstance(ctx context.Context, id egoscale.UUID) (*egoscale.Instance, error) {
	if f.getInstance == nil {
		panic("unexpected GetInstance")
	}
	return f.getInstance(ctx, id)
}

func (f *instanceExoscaleClientFake) DeleteInstance(ctx context.Context, id egoscale.UUID) (*egoscale.Operation, error) {
	if f.deleteInstance == nil {
		panic("unexpected DeleteInstance")
	}
	return f.deleteInstance(ctx, id)
}

func (f *instanceExoscaleClientFake) ListInstanceTypes(ctx context.Context) (*egoscale.ListInstanceTypesResponse, error) {
	if f.listInstanceTypes == nil {
		panic("unexpected ListInstanceTypes")
	}
	return f.listInstanceTypes(ctx)
}

func (f *instanceExoscaleClientFake) GetTemplate(ctx context.Context, id egoscale.UUID) (*egoscale.Template, error) {
	if f.getTemplate == nil {
		panic("unexpected GetTemplate")
	}
	return f.getTemplate(ctx, id)
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

func Test_cloud_UpdateElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	eip := domain.ElasticIP{
		ID:              id,
		IP:              "1.2.3.4",
		Description:     "description",
		HealthCheckPort: 12345,
	}

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, nil)
			},
		},
		{
			name: "update eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().UpdateElasticIP(ctx, egoscale.UUID(id.String()), egoscale.UpdateElasticIPRequest{
					Description: eip.Description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(eip.HealthCheckPort),
					},
				}).Return(&egoscale.Operation{}, nil)
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			err := client.UpdateElasticIP(ctx, eip)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_DeleteElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, nil)
			},
		},
		{
			name: "delete eip returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteElasticIP(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			err := client.DeleteElasticIP(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_CreateSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	name := "sg-name"

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}},
				).Return(&egoscale.Operation{}, nil)
			},
			output: id,
		},
		{
			name: "create security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.CreateSecurityGroup(ctx, name)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_GetSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    domain.SecurityGroup
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{Name: "sg-name"}, nil)
			},
			output: domain.SecurityGroup{
				ID:   id,
				Name: "sg-name",
			},
		},
		{
			name: "get security group returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{}, egoscale.ErrNotFound)
			},
			err: domain.ErrSecurityGroupNotFound,
		},
		{
			name: "get security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.GetSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_DeleteSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}).
					Return(&egoscale.Operation{}, nil)
			},
		},
		{
			name: "delete security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteSecurityGroup(ctx, egoscale.UUID(id.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			err := client.DeleteSecurityGroup(ctx, id)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_CreateSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()
	network := "10.0.0.0/8"

	baseRule := domain.SecurityGroupRule{
		Description:   "allow tcp",
		FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
		Protocol:      domain.SecurityGroupRuleProtocolTCP,
		StartPort:     80,
		EndPort:       80,
	}

	tests := []struct {
		name      string
		rule      domain.SecurityGroupRule
		exoClient func(m *mocks.ExoscaleClient)
		output    uuid.UUID
		err       error
	}{
		{
			name: "nominal - with network",
			rule: domain.SecurityGroupRule{
				Description:   baseRule.Description,
				FlowDirection: baseRule.FlowDirection,
				Protocol:      baseRule.Protocol,
				StartPort:     baseRule.StartPort,
				EndPort:       baseRule.EndPort,
				Network:       &network,
			},
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					Network:       network,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
				).Return(&egoscale.Operation{}, nil)
			},
			output: ruleID,
		},
		{
			name: "nominal - with source security group",
			rule: domain.SecurityGroupRule{
				Description:   baseRule.Description,
				FlowDirection: baseRule.FlowDirection,
				Protocol:      baseRule.Protocol,
				StartPort:     baseRule.StartPort,
				EndPort:       baseRule.EndPort,
				SecurityGroup: &sgID,
			},
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sgID.String())},
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}},
				).Return(&egoscale.Operation{}, nil)
			},
			output: ruleID,
		},
		{
			name: "add rule returned an error",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
				}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
				}).Return(
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
					nil,
				)
				m.EXPECT().Wait(
					ctx,
					&egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("123456")}},
				).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.CreateSecurityGroupRule(ctx, sgID, ut.rule)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}

func Test_cloud_DeleteSecurityGroupRule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID := uuid.New()

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		err       error
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().
					Wait(ctx, &egoscale.Operation{}).
					Return(&egoscale.Operation{}, nil)
			},
		},
		{
			name: "delete rule returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().
					DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().DeleteRuleFromSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.UUID(ruleID.String())).
					Return(&egoscale.Operation{}, nil)
				m.EXPECT().Wait(ctx, &egoscale.Operation{}).Return(&egoscale.Operation{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			err := client.DeleteSecurityGroupRule(ctx, sgID, ruleID)

			assert.ErrorIs(t, err, ut.err)
		})
	}
}

func Test_cloud_ListSecurityGroupRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sgID := uuid.New()
	ruleID1 := uuid.New()
	ruleID2 := uuid.New()
	sourceSGID := uuid.New()
	network := "10.0.0.0/8"

	tests := []struct {
		name      string
		exoClient func(m *mocks.ExoscaleClient)
		output    []domain.SecurityGroupRule
		err       error
	}{
		{
			name: "nominal - empty rules",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{Rules: []egoscale.SecurityGroupRule{}}, nil)
			},
			output: []domain.SecurityGroupRule{},
		},
		{
			name: "nominal - rule with network",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{
						Rules: []egoscale.SecurityGroupRule{
							// With network as the source
							{
								ID:            egoscale.UUID(ruleID1.String()),
								Description:   "allow tcp",
								FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
								Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
								StartPort:     80,
								EndPort:       80,
								Network:       network,
							},
							// With security group as the source
							{
								ID:            egoscale.UUID(ruleID2.String()),
								Description:   "allow from sg",
								FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
								Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
								StartPort:     443,
								EndPort:       443,
								SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sourceSGID.String())},
							},
						},
					}, nil)
			},
			output: []domain.SecurityGroupRule{
				{
					ID:            ruleID1,
					Description:   "allow tcp",
					FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      domain.SecurityGroupRuleProtocolTCP,
					StartPort:     80,
					EndPort:       80,
					Network:       &network,
				},
				{
					ID:            ruleID2,
					Description:   "allow from sg",
					FlowDirection: domain.SecurityGroupRuleFlowDirectionIngress,
					Protocol:      domain.SecurityGroupRuleProtocolTCP,
					StartPort:     443,
					EndPort:       443,
					SecurityGroup: &sourceSGID,
				},
			},
		},
		{
			name: "get security group returned not found",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{}, egoscale.ErrNotFound)
			},
			err: domain.ErrSecurityGroupNotFound,
		},
		{
			name: "get security group returned an error",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).
					Return(&egoscale.SecurityGroup{}, assert.AnError)
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			if ut.exoClient != nil {
				ut.exoClient(exoClient)
			}

			client := cloud{exoClient: exoClient}

			output, err := client.ListSecurityGroupRules(ctx, sgID)

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, output)
		})
	}
}
