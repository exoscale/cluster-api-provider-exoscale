package controller

import (
	"context"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestExoscaleMachineReconciler_Reconcile_deletesInstance(t *testing.T) {
	t.Parallel()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
		secretName          = "exoscale-creds"
	)

	ctx := context.Background()
	deleteTime := metav1.Now()
	machineUID := domain.MachineUID("machine-id")
	instanceID := uuid.New()
	clusterID := uuid.New()
	controllerOwner := true

	tests := []struct {
		name             string
		ownerReference   bool
		ownerMachine     bool
		paused           bool
		statusInstanceID string
		deleteErr        error
		wantService      bool
		wantErr          error
		wantFinalizer    bool
	}{
		{name: "recovers instance by Machine UID", ownerReference: true, ownerMachine: true, wantService: true},
		{name: "missing owner without instance removes finalizer", ownerReference: true},
		{name: "status instance without owner keeps finalizer", statusInstanceID: instanceID.String(), deleteErr: assert.AnError, wantService: true, wantErr: assert.AnError, wantFinalizer: true},
		{name: "without either identifier removes finalizer"},
		{name: "instance not found removes finalizer", ownerReference: true, ownerMachine: true, statusInstanceID: instanceID.String(), deleteErr: domain.ErrInstanceNotFound, wantService: true},
		{name: "delete error keeps finalizer", ownerReference: true, ownerMachine: true, statusInstanceID: instanceID.String(), deleteErr: assert.AnError, wantService: true, wantErr: assert.AnError, wantFinalizer: true},
		{name: "paused deletion keeps finalizer", ownerReference: true, paused: true, statusInstanceID: instanceID.String(), wantFinalizer: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := newExoscaleMachineTestScheme(t)
			instanceSvc := mocks.NewInstanceService(t)
			var expectedMachineUID domain.MachineUID
			var expectedInstanceID *uuid.UUID
			if tc.ownerMachine {
				expectedMachineUID = machineUID
			}
			if tc.statusInstanceID != "" {
				expectedInstanceID = &instanceID
			}
			if tc.wantService {
				instanceSvc.EXPECT().DeleteInstance(ctx, expectedMachineUID, clusterID, expectedInstanceID).Return(tc.deleteErr)
			}

			objects := []crclient.Object{
				&clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: infrav1alpha1.GroupVersion.Group,
							Kind:     "ExoscaleCluster",
							Name:     exoscaleClusterName,
						},
					},
				},
				&infrav1alpha1.ExoscaleCluster{
					ObjectMeta: metav1.ObjectMeta{Name: exoscaleClusterName, Namespace: ns},
					Spec: infrav1alpha1.ExoscaleClusterSpec{
						Zone: "ch-gva-2",
						ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
							Name:      secretName,
							ApiKey:    "apikey",
							APISecret: "apisecret",
						},
					},
					Status: infrav1alpha1.ExoscaleClusterStatus{ID: new(clusterID.String())},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
					Data: map[string][]byte{
						"apikey":    []byte("my-api-key"),
						"apisecret": []byte("my-api-secret"),
					},
				},
			}
			if tc.ownerMachine {
				objects = append(objects, &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      machineName,
						Namespace: ns,
						UID:       types.UID(machineUID),
						Labels:    map[string]string{clusterv1.ClusterNameLabel: clusterName},
					},
				})
			}

			exoMachine := &infrav1alpha1.ExoscaleMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              exoscaleMachineName,
					Namespace:         ns,
					Finalizers:        []string{machineFinalizer},
					DeletionTimestamp: &deleteTime,
					Labels:            map[string]string{clusterv1.ClusterNameLabel: clusterName},
				},
				Status: infrav1alpha1.ExoscaleMachineStatus{InstanceID: tc.statusInstanceID},
			}
			if tc.ownerReference {
				exoMachine.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       machineName,
					UID:        types.UID(machineUID),
					Controller: &controllerOwner,
				}}
			}
			if tc.paused {
				exoMachine.Annotations = map[string]string{clusterv1.PausedAnnotation: ""}
			}
			objects = append(objects, exoMachine)

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&infrav1alpha1.ExoscaleMachine{}).
				WithObjects(objects...).
				Build()

			r := &ExoscaleMachineReconciler{
				Client: client,
				Scheme: scheme,
				NewInstanceService: func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
					if !tc.wantService {
						assert.Fail(t, "instance service should not be created without a deletion identifier")
					}
					return instanceSvc, nil
				},
			}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

			assert.Equal(t, reconcile.Result{}, result)
			assert.ErrorIs(t, err, tc.wantErr)

			updated := &infrav1alpha1.ExoscaleMachine{}
			getErr := client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated)
			if !tc.wantFinalizer && apierrors.IsNotFound(getErr) {
				return
			}
			assert.NoError(t, getErr)
			if tc.wantFinalizer {
				assert.Contains(t, updated.Finalizers, machineFinalizer)
			} else {
				assert.NotContains(t, updated.Finalizers, machineFinalizer)
			}
			if tc.paused {
				assert.NotNil(t, apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.PausedCondition))
				assert.Nil(t, apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.ReadyCondition))
			}
		})
	}
}

func Test_deletionInstanceIDs(t *testing.T) {
	t.Parallel()

	instanceID := uuid.New()
	machineUID := domain.MachineUID("not-a-uuid")
	providerID := "exoscale://" + instanceID.String()
	legacyProviderID := "exoscale:///" + instanceID.String()
	invalidProviderID := "bad-id"
	tests := []struct {
		name           string
		exoMachine     infrav1alpha1.ExoscaleMachine
		machine        *clusterv1.Machine
		wantMachineUID domain.MachineUID
		wantInstanceID *uuid.UUID
		wantErr        string
	}{
		{
			name:       "invalid status instance ID",
			exoMachine: infrav1alpha1.ExoscaleMachine{Status: infrav1alpha1.ExoscaleMachineStatus{InstanceID: "bad-id"}},
			wantErr:    "invalid instanceID in status",
		},
		{
			name:           "recovers instance ID from provider ID after move",
			exoMachine:     infrav1alpha1.ExoscaleMachine{Spec: infrav1alpha1.ExoscaleMachineSpec{ProviderID: &providerID}},
			wantInstanceID: &instanceID,
		},
		{
			name:           "accepts provider ID with path separator",
			exoMachine:     infrav1alpha1.ExoscaleMachine{Spec: infrav1alpha1.ExoscaleMachineSpec{ProviderID: &legacyProviderID}},
			wantInstanceID: &instanceID,
		},
		{
			name:       "invalid provider ID",
			exoMachine: infrav1alpha1.ExoscaleMachine{Spec: infrav1alpha1.ExoscaleMachineSpec{ProviderID: &invalidProviderID}},
			wantErr:    "invalid providerID",
		},
		{
			name:           "accepts non-UUID Machine UID",
			machine:        &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{UID: types.UID(machineUID)}},
			wantMachineUID: machineUID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMachineUID, gotInstanceID, err := getDeletionInstanceIDs(tc.machine, &tc.exoMachine)

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantMachineUID, gotMachineUID)
			assert.Equal(t, tc.wantInstanceID, gotInstanceID)
		})
	}
}
