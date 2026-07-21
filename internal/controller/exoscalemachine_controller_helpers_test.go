package controller

import (
	"context"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestExoscaleMachineReconciler_getExoscaleCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newExoscaleMachineTestScheme(t)
	r := &ExoscaleMachineReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}

	t.Run("requires infrastructure reference", func(t *testing.T) {
		cluster, err := r.getExoscaleCluster(ctx, &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}})

		assert.Nil(t, cluster)
		assert.ErrorContains(t, err, `cluster "cluster" has no infrastructureRef`)
	})

	t.Run("wraps missing ExoscaleCluster", func(t *testing.T) {
		cluster, err := r.getExoscaleCluster(ctx, &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
			Spec: clusterv1.ClusterSpec{InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: infrav1alpha1.GroupVersion.Group,
				Kind:     "ExoscaleCluster",
				Name:     "missing",
			}},
		})

		assert.Nil(t, cluster)
		assert.ErrorContains(t, err, `unable to fetch ExoscaleCluster "missing"`)
		assert.True(t, apierrors.IsNotFound(err))
	})
}

func TestExoscaleMachineReconciler_instanceService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const secretName = "credentials"
	exoCluster := &infrav1alpha1.ExoscaleCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: infrav1alpha1.ExoscaleClusterSpec{
			Zone: "ch-gva-2",
			ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
				Name: secretName, ApiKey: "apikey", APISecret: "apisecret",
			},
		},
	}
	tests := []struct {
		name             string
		wireFactory      bool
		secretData       map[string][]byte
		factoryErr       error
		wantErr          string
		wantNotFound     bool
		wantInvalidCreds bool
	}{
		{name: "requires factory", wantErr: "NewInstanceService is not wired"},
		{name: "wraps missing Secret", wireFactory: true, wantErr: "unable to fetch exoscale secret", wantNotFound: true},
		{name: "rejects missing API secret", wireFactory: true, secretData: map[string][]byte{"apikey": []byte("key")}, wantErr: "unable to find api secret", wantInvalidCreds: true},
		{name: "returns factory error", wireFactory: true, secretData: map[string][]byte{"apikey": []byte("key"), "apisecret": []byte("secret")}, factoryErr: assert.AnError, wantErr: assert.AnError.Error()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := newExoscaleMachineTestScheme(t)
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.secretData != nil {
				builder = builder.WithObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
					Data:       tc.secretData,
				})
			}
			r := &ExoscaleMachineReconciler{Client: builder.Build(), Scheme: scheme}
			if tc.wireFactory {
				r.NewInstanceService = func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
					if tc.factoryErr == nil {
						assert.Fail(t, "instance service factory should not be called")
					}
					return nil, tc.factoryErr
				}
			}

			service, err := r.instanceService(ctx, exoCluster)

			assert.Nil(t, service)
			assert.ErrorContains(t, err, tc.wantErr)
			if tc.wantNotFound {
				assert.True(t, apierrors.IsNotFound(err))
			}
			if tc.wantInvalidCreds {
				assert.ErrorIs(t, err, errInvalidCreds)
			}
			if tc.factoryErr != nil {
				assert.ErrorIs(t, err, tc.factoryErr)
			}
		})
	}
}

func TestExoscaleMachineReconciler_bootstrapData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	missingName := "missing"
	emptyName := "empty"
	emptyValueName := "empty-value"
	tests := []struct {
		name         string
		secretName   *string
		secret       *corev1.Secret
		wantErr      string
		wantNotFound bool
	}{
		{name: "requires secret name", wantErr: "bootstrap data secret name is not set"},
		{name: "wraps missing Secret", secretName: &missingName, wantErr: `unable to fetch bootstrap data secret "missing"`, wantNotFound: true},
		{
			name:       "requires value key",
			secretName: &emptyName,
			secret:     &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: emptyName, Namespace: "default"}},
			wantErr:    `bootstrap data secret "empty" has no value key`,
		},
		{
			name:       "requires non-empty value",
			secretName: &emptyValueName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: emptyValueName, Namespace: "default"},
				Data:       map[string][]byte{"value": {}},
			},
			wantErr: `bootstrap data secret "empty-value" has an empty value`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := newExoscaleMachineTestScheme(t)
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.secret != nil {
				builder = builder.WithObjects(tc.secret)
			}
			r := &ExoscaleMachineReconciler{Client: builder.Build(), Scheme: scheme}
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: tc.secretName}},
			}

			data, err := r.bootstrapData(ctx, machine)

			assert.Empty(t, data)
			assert.ErrorContains(t, err, tc.wantErr)
			if tc.wantNotFound {
				assert.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}

func Test_securityGroupIDs_rejectsInvalidIDs(t *testing.T) {
	t.Parallel()

	validID := uuid.New().String()

	tests := map[string]struct {
		cluster      infrav1alpha1.ExoscaleCluster
		machine      infrav1alpha1.ExoscaleMachine
		controlPlane bool
		wantErr      string
	}{
		"invalid node security group": {
			cluster: infrav1alpha1.ExoscaleCluster{
				Status: infrav1alpha1.ExoscaleClusterStatus{SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{ID: "bad-id"}},
			},
			wantErr: "invalid node security group ID",
		},
		"invalid control plane security group": {
			cluster: infrav1alpha1.ExoscaleCluster{
				Status: infrav1alpha1.ExoscaleClusterStatus{SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: "bad-id"}},
			},
			controlPlane: true,
			wantErr:      "invalid control plane security group ID",
		},
		"invalid machine security group": {
			cluster: infrav1alpha1.ExoscaleCluster{
				Status: infrav1alpha1.ExoscaleClusterStatus{SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{ID: validID}},
			},
			machine: infrav1alpha1.ExoscaleMachine{Spec: infrav1alpha1.ExoscaleMachineSpec{SecurityGroups: []string{"bad-id"}}},
			wantErr: "invalid ExoscaleMachine security group ID",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := securityGroupIDs(&tc.cluster, &tc.machine, tc.controlPlane)

			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func Test_securityGroupIDs_selectsMachineRole(t *testing.T) {
	t.Parallel()

	controlPlaneID := uuid.New()
	nodeID := uuid.New()
	customID := uuid.New()
	cluster := infrav1alpha1.ExoscaleCluster{Status: infrav1alpha1.ExoscaleClusterStatus{
		SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: controlPlaneID.String()},
		SecurityGroupNode:        &infrav1alpha1.SecurityGroupStatus{ID: nodeID.String()},
	}}
	machine := infrav1alpha1.ExoscaleMachine{Spec: infrav1alpha1.ExoscaleMachineSpec{SecurityGroups: []string{customID.String()}}}

	controlPlaneIDs, err := securityGroupIDs(&cluster, &machine, true)
	assert.NoError(t, err)
	assert.Equal(t, []uuid.UUID{controlPlaneID, customID}, controlPlaneIDs)

	nodeIDs, err := securityGroupIDs(&cluster, &machine, false)
	assert.NoError(t, err)
	assert.Equal(t, []uuid.UUID{nodeID, customID}, nodeIDs)
}
