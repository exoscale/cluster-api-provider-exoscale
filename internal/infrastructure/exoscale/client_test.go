package exoscale

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	"github.com/stretchr/testify/assert"

	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/go-logr/logr/funcr"
	"github.com/google/uuid"
)

func Test_traceAPIRequest_omitsSensitiveData(t *testing.T) {
	t.Parallel()

	var output string
	logger := funcr.New(func(_, args string) { output += args }, funcr.Options{})
	req, err := http.NewRequest(http.MethodPost, "https://api.example.test/v2/instance?token=query-secret", strings.NewReader("body-secret"))
	assert.NoError(t, err)
	req.Header.Set("Authorization", "header-secret")

	assert.NoError(t, traceAPIRequest(logger)(context.Background(), req))
	assert.Contains(t, output, "Exoscale API request")
	assert.Contains(t, output, "POST")
	assert.Contains(t, output, "/v2/instance")
	assert.NotContains(t, output, "query-secret")
	assert.NotContains(t, output, "header-secret")
	assert.NotContains(t, output, "body-secret")
}

func Test_NewCloud_rejectsIncompleteCredentials(t *testing.T) {
	t.Parallel()

	client, err := NewCloud("key", "", egoscale.ZoneNameCHGva2, funcr.New(func(_, _ string) {}, funcr.Options{}), false)

	assert.Nil(t, client)
	assert.ErrorIs(t, err, credentials.ErrMissingIncomplete)
	assert.ErrorContains(t, err, "unable to create exoscale client")
}

func Test_cloud_waitForSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := &egoscale.Operation{State: egoscale.OperationStatePending}
	tests := []struct {
		name      string
		completed *egoscale.Operation
		waitErr   error
		wantErr   string
	}{
		{name: "success", completed: &egoscale.Operation{State: egoscale.OperationStateSuccess}},
		{name: "wait error", waitErr: assert.AnError, wantErr: assert.AnError.Error()},
		{name: "nil operation", wantErr: "operation did not succeed"},
		{name: "failed operation", completed: &egoscale.Operation{State: egoscale.OperationStateFailure}, wantErr: "operation did not succeed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(tc.completed, tc.waitErr)
			client := cloud{exoClient: exoClient}

			completed, err := client.waitForSuccess(ctx, op)

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, completed)
			} else {
				assert.NoError(t, err)
				assert.Same(t, tc.completed, completed)
			}
		})
	}
}

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
		errText   string
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{
						Mode: egoscale.ElasticIPHealthcheckModeTCP,
						Port: int64(healthCheckPort),
					},
				}).Return(op, nil)
				m.EXPECT().Wait(
					ctx,
					op,
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}}, nil)
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
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned no reference",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{Mode: egoscale.ElasticIPHealthcheckModeTCP, Port: int64(healthCheckPort)},
				}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
			errText: "returned no reference",
		},
		{
			name: "wait returned invalid reference",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateElasticIP(ctx, egoscale.CreateElasticIPRequest{
					Description: description,
					Healthcheck: &egoscale.ElasticIPHealthcheck{Mode: egoscale.ElasticIPHealthcheckModeTCP, Port: int64(healthCheckPort)},
				}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: "not-a-uuid"}}, nil)
			},
			errText: "unable to parse response from create elastic ip",
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

			if ut.errText != "" {
				assert.ErrorContains(t, err, ut.errText)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
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
		errText   string
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
		{
			name: "invalid elastic IP ID",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().ListElasticIPS(ctx).Return(&egoscale.ListElasticIPSResponse{
					ElasticIPS: []egoscale.ElasticIP{{ID: "not-a-uuid"}},
				}, nil)
			},
			errText: "unable to parse elastic IP ID",
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

			if ut.errText != "" {
				assert.ErrorContains(t, err, ut.errText)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
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
	instanceTypeID := egoscale.UUID(uuid.New().String())
	instanceType := egoscale.InstanceType{
		ID:     instanceTypeID,
		Family: egoscale.InstanceTypeFamilyStandard,
		Size:   egoscale.InstanceTypeSize("2"),
	}
	spec := domain.ResolvedInstanceSpec{
		Name:             "machine-0",
		TemplateID:       templateID,
		InstanceType:     domain.InstanceType{ID: instanceTypeID.String(), Family: "standard", Size: "2"},
		SSHKey:           "ssh-key",
		SecurityGroupIDs: []uuid.UUID{securityGroupID},
		DiskSizeGB:       20,
		UserData:         "#cloud-config",
		Labels:           map[string]string{"machine": "uid"},
	}

	createOp := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
	exoClient := mocks.NewExoscaleClient(t)
	instanceClient := mocks.NewInstanceClient(t)
	instanceClient.EXPECT().CreateInstance(ctx, egoscale.CreateInstanceRequest{
		DiskSize:           int64(20),
		InstanceType:       &instanceType,
		Labels:             egoscale.Labels{"machine": "uid"},
		Name:               "machine-0",
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
		SSHKey:             &egoscale.SSHKey{Name: "ssh-key"},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
		UserData:           base64.StdEncoding.EncodeToString([]byte("#cloud-config")),
	}).Return(createOp, nil)
	exoClient.EXPECT().Wait(ctx, createOp, []egoscale.OperationState{egoscale.OperationStateSuccess}).
		Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: createOp.Reference}, nil)

	client := cloud{exoClient: exoClient, instanceClient: instanceClient}

	output, err := client.CreateInstance(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, instanceID, output)
}

func Test_cloud_ListInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	securityGroupID := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	t.Run("maps instances", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().ListInstances(ctx).
			Return(&egoscale.ListInstancesResponse{Instances: []egoscale.ListInstancesResponseInstances{{
				ID:             egoscale.UUID(id.String()),
				Name:           "machine-0",
				State:          egoscale.InstanceStateRunning,
				PublicIP:       net.ParseIP("1.2.3.4"),
				CreatedAT:      createdAt,
				Labels:         egoscale.Labels{"machine": "uid"},
				SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
			}}}, nil)

		client := cloud{instanceClient: exoClient}

		output, err := client.ListInstances(ctx)

		assert.NoError(t, err)
		assert.Equal(t, []domain.Instance{{
			ID:               id,
			Name:             "machine-0",
			State:            "running",
			PublicIP:         "1.2.3.4",
			CreatedAt:        createdAt.Format(time.RFC3339),
			Labels:           map[string]string{"machine": "uid"},
			SecurityGroupIDs: []uuid.UUID{securityGroupID},
		}}, output)
	})

	t.Run("returns list error", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().ListInstances(ctx).Return(nil, assert.AnError)

		client := cloud{instanceClient: exoClient}

		output, err := client.ListInstances(ctx)

		assert.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, output)
	})

	t.Run("rejects invalid instance ID", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().ListInstances(ctx).Return(&egoscale.ListInstancesResponse{
			Instances: []egoscale.ListInstancesResponseInstances{{ID: "not-a-uuid"}},
		}, nil)

		output, err := (&cloud{instanceClient: exoClient}).ListInstances(ctx)

		assert.Nil(t, output)
		assert.ErrorContains(t, err, "unable to parse instance ID")
	})

	t.Run("rejects invalid instance security group ID", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().ListInstances(ctx).Return(&egoscale.ListInstancesResponse{
			Instances: []egoscale.ListInstancesResponseInstances{{
				ID:             egoscale.UUID(id.String()),
				SecurityGroups: []egoscale.SecurityGroup{{ID: "not-a-uuid"}},
			}},
		}, nil)

		output, err := (&cloud{instanceClient: exoClient}).ListInstances(ctx)

		assert.Nil(t, output)
		assert.ErrorContains(t, err, "unable to parse instance security group ID")
	})
}

func Test_cloud_GetInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	securityGroupID := uuid.New()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	t.Run("maps instance", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).
			Return(&egoscale.Instance{
				Name:           "machine-0",
				State:          egoscale.InstanceStateRunning,
				PublicIP:       net.ParseIP("1.2.3.4"),
				CreatedAT:      createdAt,
				Labels:         egoscale.Labels{"machine": "uid"},
				SecurityGroups: []egoscale.SecurityGroup{{ID: egoscale.UUID(securityGroupID.String())}},
			}, nil)

		client := cloud{instanceClient: exoClient}

		output, err := client.GetInstance(ctx, id)

		assert.NoError(t, err)
		assert.Equal(t, domain.Instance{
			ID:               id,
			Name:             "machine-0",
			State:            "running",
			PublicIP:         "1.2.3.4",
			CreatedAt:        createdAt.Format(time.RFC3339),
			Labels:           map[string]string{"machine": "uid"},
			SecurityGroupIDs: []uuid.UUID{securityGroupID},
		}, output)
	})

	t.Run("maps not found", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

		client := cloud{instanceClient: exoClient}

		_, err := client.GetInstance(ctx, id)

		assert.ErrorIs(t, err, domain.ErrInstanceNotFound)
	})

	t.Run("returns get error", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(nil, assert.AnError)

		client := cloud{instanceClient: exoClient}

		_, err := client.GetInstance(ctx, id)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("rejects invalid instance security group ID", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewInstanceClient(t)
		exoClient.EXPECT().GetInstance(ctx, egoscale.UUID(id.String())).Return(&egoscale.Instance{
			SecurityGroups: []egoscale.SecurityGroup{{ID: "not-a-uuid"}},
		}, nil)

		output, err := (&cloud{instanceClient: exoClient}).GetInstance(ctx, id)

		assert.Equal(t, domain.Instance{}, output)
		assert.ErrorContains(t, err, "unable to parse instance security group ID")
	})
}

func Test_cloud_AttachInstanceToElasticIP(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	elasticIPID := uuid.New()
	request := egoscale.AttachInstanceToElasticIPRequest{
		Instance: &egoscale.InstanceTarget{ID: egoscale.UUID(instanceID.String())},
	}
	op := &egoscale.Operation{ID: egoscale.UUID(uuid.New().String())}

	t.Run("attaches and waits", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
			Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.NoError(t, client.AttachInstanceToElasticIP(ctx, instanceID, elasticIPID))
	})

	t.Run("returns attach error", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), request).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		assert.ErrorIs(t, client.AttachInstanceToElasticIP(ctx, instanceID, elasticIPID), assert.AnError)
	})

	t.Run("returns wait error", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorIs(t, client.AttachInstanceToElasticIP(ctx, instanceID, elasticIPID), assert.AnError)
	})

	t.Run("rejects unsuccessful operation", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToElasticIP(ctx, egoscale.UUID(elasticIPID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
			Return(&egoscale.Operation{State: egoscale.OperationStateFailure}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorContains(t, client.AttachInstanceToElasticIP(ctx, instanceID, elasticIPID), "did not succeed")
	})
}

func Test_cloud_AttachInstanceToSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()
	request := egoscale.AttachInstanceToSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	}
	op := &egoscale.Operation{ID: egoscale.UUID(uuid.NewString())}

	t.Run("attaches and waits", func(t *testing.T) {
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
			Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.NoError(t, client.AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID))
	})

	t.Run("returns attach error", func(t *testing.T) {
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		assert.ErrorIs(t, client.AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID), assert.AnError)
	})

	t.Run("returns wait error", func(t *testing.T) {
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorIs(t, client.AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID), assert.AnError)
	})

	t.Run("rejects unsuccessful operation", func(t *testing.T) {
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().AttachInstanceToSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
			Return(&egoscale.Operation{State: egoscale.OperationStateFailure}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorContains(t, client.AttachInstanceToSecurityGroup(ctx, instanceID, securityGroupID), "did not succeed")
	})
}

func Test_cloud_DetachInstanceFromSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	securityGroupID := uuid.New()
	request := egoscale.DetachInstanceFromSecurityGroupRequest{
		Instance: &egoscale.Instance{ID: egoscale.UUID(instanceID.String())},
	}
	op := &egoscale.Operation{ID: egoscale.UUID(uuid.NewString())}

	t.Run("detaches and waits", func(t *testing.T) {
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
			Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.NoError(t, client.DetachInstanceFromSecurityGroup(ctx, instanceID, securityGroupID))
	})

	t.Run("returns detach error", func(t *testing.T) {
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		assert.ErrorIs(t, client.DetachInstanceFromSecurityGroup(ctx, instanceID, securityGroupID), assert.AnError)
	})

	tests := []struct {
		name      string
		completed *egoscale.Operation
		waitErr   error
		errText   string
	}{
		{name: "returns wait error", waitErr: assert.AnError, errText: "error while waiting for instance detachment from security group"},
		{name: "rejects nil operation", errText: "instance detachment from security group operation did not succeed"},
		{name: "rejects unsuccessful operation", completed: &egoscale.Operation{State: egoscale.OperationStateFailure}, errText: "instance detachment from security group operation did not succeed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exoClient := mocks.NewExoscaleClient(t)
			instanceClient := mocks.NewInstanceClient(t)
			instanceClient.EXPECT().DetachInstanceFromSecurityGroup(ctx, egoscale.UUID(securityGroupID.String()), request).Return(op, nil)
			exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(tc.completed, tc.waitErr)

			client := cloud{exoClient: exoClient, instanceClient: instanceClient}
			err := client.DetachInstanceFromSecurityGroup(ctx, instanceID, securityGroupID)

			assert.ErrorContains(t, err, tc.errText)
			if tc.waitErr != nil {
				assert.ErrorIs(t, err, tc.waitErr)
			}
		})
	}
}

func Test_cloud_DeleteInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()
	op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}}

	t.Run("deletes and waits", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.NoError(t, client.DeleteInstance(ctx, id))
	})

	t.Run("returns delete error", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorIs(t, client.DeleteInstance(ctx, id), assert.AnError)
	})

	t.Run("rejects terminal delete failure", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{State: egoscale.OperationStateFailure}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorContains(t, client.DeleteInstance(ctx, id), "did not succeed")
	})

	t.Run("maps delete not found", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(nil, egoscale.ErrNotFound)

		client := cloud{instanceClient: instanceClient}

		assert.ErrorIs(t, client.DeleteInstance(ctx, id), domain.ErrInstanceNotFound)
	})

	t.Run("returns wait error", func(t *testing.T) {
		t.Parallel()

		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().DeleteInstance(ctx, egoscale.UUID(id.String())).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		assert.ErrorIs(t, client.DeleteInstance(ctx, id), assert.AnError)
	})
}

func Test_cloud_CreateInstance_errors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	templateID := uuid.New()
	instanceTypeID := uuid.New().String()
	spec := domain.ResolvedInstanceSpec{
		TemplateID:   templateID,
		InstanceType: domain.InstanceType{ID: instanceTypeID, Family: "standard", Size: "2"},
		DiskSizeGB:   10,
	}
	req := egoscale.CreateInstanceRequest{
		DiskSize: int64(10),
		InstanceType: &egoscale.InstanceType{
			ID:     egoscale.UUID(instanceTypeID),
			Family: egoscale.InstanceTypeFamilyStandard,
			Size:   egoscale.InstanceTypeSize("2"),
		},
		Labels:             egoscale.Labels(nil),
		PublicIPAssignment: egoscale.PublicIPAssignmentInet4,
		SecurityGroups:     []egoscale.SecurityGroup{},
		Template:           &egoscale.Template{ID: egoscale.UUID(templateID.String())},
	}

	t.Run("returns create error", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().CreateInstance(ctx, req).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("returns wait error", func(t *testing.T) {
		t.Parallel()

		op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().CreateInstance(ctx, req).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("returns operation id parse error", func(t *testing.T) {
		t.Parallel()

		op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID("not-a-uuid")}}
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().CreateInstance(ctx, req).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: op.Reference}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorContains(t, err, "unable to parse response from create instance")
	})

	t.Run("rejects missing operation reference", func(t *testing.T) {
		t.Parallel()

		op := &egoscale.Operation{}
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().CreateInstance(ctx, req).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorContains(t, err, "returned no reference")
	})

	t.Run("rejects terminal create failure", func(t *testing.T) {
		t.Parallel()

		op := &egoscale.Operation{Reference: &egoscale.OperationReference{ID: egoscale.UUID(instanceID.String())}}
		exoClient := mocks.NewExoscaleClient(t)
		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().CreateInstance(ctx, req).Return(op, nil)
		exoClient.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(&egoscale.Operation{State: egoscale.OperationStateFailure}, nil)

		client := cloud{exoClient: exoClient, instanceClient: instanceClient}

		_, err := client.CreateInstance(ctx, spec)

		assert.ErrorContains(t, err, "did not succeed")
	})
}

func Test_cloud_ListInstanceTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuid.New()

	t.Run("maps instance types", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().ListInstanceTypes(ctx).Return(&egoscale.ListInstanceTypesResponse{InstanceTypes: []egoscale.InstanceType{{
			ID:     egoscale.UUID(id.String()),
			Family: egoscale.InstanceTypeFamilyStandard,
			Size:   egoscale.InstanceTypeSize("2"),
		}}}, nil)

		client := cloud{instanceClient: instanceClient}

		output, err := client.ListInstanceTypes(ctx)

		assert.NoError(t, err)
		assert.Equal(t, []domain.InstanceType{{ID: id.String(), Family: "standard", Size: "2"}}, output)
	})

	t.Run("returns list error", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().ListInstanceTypes(ctx).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		output, err := client.ListInstanceTypes(ctx)

		assert.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, output)
	})
}

func Test_cloud_GetTemplate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	createdAt := time.Now().UTC()

	t.Run("maps template", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().GetTemplate(ctx, egoscale.UUID(templateID.String())).Return(&egoscale.Template{
			ID:        egoscale.UUID(templateID.String()),
			Name:      "ubuntu",
			Size:      15,
			CreatedAT: createdAt,
		}, nil)

		client := cloud{instanceClient: instanceClient}

		output, err := client.GetTemplate(ctx, templateID)

		assert.NoError(t, err)
		assert.Equal(t, domain.InstanceTemplate{ID: templateID, Name: "ubuntu", SizeBytes: 15, CreatedAt: createdAt}, output)
	})

	t.Run("returns get error", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().GetTemplate(ctx, egoscale.UUID(templateID.String())).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		output, err := client.GetTemplate(ctx, templateID)

		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, domain.InstanceTemplate{}, output)
	})
}

func Test_cloud_ListTemplates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	templateID := uuid.New()
	createdAt := time.Now().UTC()

	t.Run("maps templates", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().ListTemplates(ctx).Return(&egoscale.ListTemplatesResponse{Templates: []egoscale.Template{{
			ID:        egoscale.UUID(templateID.String()),
			Name:      "ubuntu",
			Size:      15,
			CreatedAT: createdAt,
		}}}, nil)

		client := cloud{instanceClient: instanceClient}

		output, err := client.ListTemplates(ctx)

		assert.NoError(t, err)
		assert.Equal(t, []domain.InstanceTemplate{{ID: templateID, Name: "ubuntu", SizeBytes: 15, CreatedAt: createdAt}}, output)
	})

	t.Run("returns list error", func(t *testing.T) {
		t.Parallel()

		instanceClient := mocks.NewInstanceClient(t)
		instanceClient.EXPECT().ListTemplates(ctx).Return(nil, assert.AnError)

		client := cloud{instanceClient: instanceClient}

		output, err := client.ListTemplates(ctx)

		assert.ErrorIs(t, err, assert.AnError)
		assert.Nil(t, output)
	})
}

func Test_mapTemplate_rejectsInvalidID(t *testing.T) {
	t.Parallel()

	template, err := mapTemplate(egoscale.Template{ID: "not-a-uuid"})

	assert.Equal(t, domain.InstanceTemplate{}, template)
	assert.ErrorContains(t, err, `unable to parse template id "not-a-uuid"`)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)
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
		errText   string
	}{
		{
			name: "nominal",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{
					Name: name,
				}).Return(op, nil)
				m.EXPECT().Wait(
					ctx,
					op,
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: egoscale.UUID(id.String())}}, nil)
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
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned no reference",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{Name: name}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
			errText: "returned no reference",
		},
		{
			name: "wait returned invalid reference",
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().CreateSecurityGroup(ctx, egoscale.CreateSecurityGroupRequest{Name: name}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: "not-a-uuid"}}, nil)
			},
			errText: "unable to parse response from create security group",
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

			if ut.errText != "" {
				assert.ErrorContains(t, err, ut.errText)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
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
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
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
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)
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
		errText   string
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
				op := &egoscale.Operation{}
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					Network:       network,
				}).Return(op, nil)
				m.EXPECT().Wait(
					ctx,
					op,
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}}, nil)
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
				op := &egoscale.Operation{}
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description:   baseRule.Description,
					FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol:      egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol),
					StartPort:     baseRule.StartPort,
					EndPort:       baseRule.EndPort,
					SecurityGroup: &egoscale.SecurityGroupResource{ID: egoscale.UUID(sgID.String())},
				}).Return(op, nil)
				m.EXPECT().Wait(
					ctx,
					op,
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: egoscale.UUID(ruleID.String())}}, nil)
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
					[]egoscale.OperationState{egoscale.OperationStateSuccess},
				).Return(nil, assert.AnError)
			},
			err: assert.AnError,
		},
		{
			name: "wait returned no reference",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description: baseRule.Description, FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol: egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol), StartPort: baseRule.StartPort, EndPort: baseRule.EndPort,
				}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
			},
			errText: "returned no reference",
		},
		{
			name: "wait returned invalid reference",
			rule: baseRule,
			exoClient: func(m *mocks.ExoscaleClient) {
				op := &egoscale.Operation{}
				m.EXPECT().AddRuleToSecurityGroup(ctx, egoscale.UUID(sgID.String()), egoscale.AddRuleToSecurityGroupRequest{
					Description: baseRule.Description, FlowDirection: egoscale.AddRuleToSecurityGroupRequestFlowDirection(baseRule.FlowDirection),
					Protocol: egoscale.AddRuleToSecurityGroupRequestProtocol(baseRule.Protocol), StartPort: baseRule.StartPort, EndPort: baseRule.EndPort,
				}).Return(op, nil)
				m.EXPECT().Wait(ctx, op, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess, Reference: &egoscale.OperationReference{ID: "not-a-uuid"}}, nil)
			},
			errText: "unable to parse response from create security group rule",
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

			if ut.errText != "" {
				assert.ErrorContains(t, err, ut.errText)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
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
					Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).
					Return(&egoscale.Operation{State: egoscale.OperationStateSuccess}, nil)
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
				m.EXPECT().Wait(ctx, &egoscale.Operation{}, []egoscale.OperationState{egoscale.OperationStateSuccess}).Return(nil, assert.AnError)
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
		errText   string
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
		{
			name: "invalid rule ID",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).Return(&egoscale.SecurityGroup{
					Rules: []egoscale.SecurityGroupRule{{ID: "not-a-uuid"}},
				}, nil)
			},
			errText: "unable to parse security group rule ID",
		},
		{
			name: "invalid source security group ID",
			exoClient: func(m *mocks.ExoscaleClient) {
				m.EXPECT().GetSecurityGroup(ctx, egoscale.UUID(sgID.String())).Return(&egoscale.SecurityGroup{
					Rules: []egoscale.SecurityGroupRule{{
						ID:            egoscale.UUID(ruleID1.String()),
						SecurityGroup: &egoscale.SecurityGroupResource{ID: "not-a-uuid"},
					}},
				}, nil)
			},
			errText: "unable to parse security group rule source ID",
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

			if ut.errText != "" {
				assert.ErrorContains(t, err, ut.errText)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
			assert.Equal(t, ut.output, output)
		})
	}
}
