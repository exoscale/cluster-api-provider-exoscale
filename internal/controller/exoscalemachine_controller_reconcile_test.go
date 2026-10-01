package controller

import (
	"context"
	"testing"
	"time"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestExoscaleMachineReconciler_Reconcile_wiresInstanceService(t *testing.T) {
	t.Parallel()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
		secretName          = "exoscale-creds"
		bootstrapSecretName = "bootstrap-data"
		template            = "Ubuntu LTS"
	)

	// TODO: rename templateID en template e.g., Ubuntu LTS
	// TODO: annotations have been banjaxed, to adjust accordingly
	ctx := context.Background()
	machineUID := uuid.New()
	instanceID := uuid.New()
	elasticIPID := uuid.New()
	controlPlaneSecurityGroupID := uuid.New()
	workerSecurityGroupID := uuid.New()
	clusterID := uuid.NewString()
	rootVolumeSize := int64(20)
	dataSecretName := bootstrapSecretName
	clusterProvisioned := true

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	instanceSvc := mocks.NewInstanceService(t)
	instanceSvc.EXPECT().UpsertInstance(ctx, domain.MachineUID(machineUID.String()), (*uuid.UUID)(nil), domain.InstanceSpec{
		Name:              machineUID.String(),
		Template:          template,
		InstanceType:      "standard.small",
		SSHKey:            "ssh-key",
		SecurityGroupIDs:  []uuid.UUID{controlPlaneSecurityGroupID},
		ElasticIPID:       &elasticIPID,
		RootVolumeSizeGiB: &rootVolumeSize,
		UserData:          "#cloud-config",
		Labels: map[string]string{
			instanceClusterIDLabel: clusterID,
			instanceRoleLabel:      string(MachineRoleControlPlane),
		},
	}).Return(domain.Instance{ID: instanceID, State: "running", PublicIP: "1.2.3.4", PrivateIP: "10.0.0.1"}, nil)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&infrav1alpha1.ExoscaleMachine{}).
		WithObjects(
			&clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: infrav1alpha1.GroupVersion.Group,
						Kind:     "ExoscaleCluster",
						Name:     exoscaleClusterName,
					},
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: &clusterProvisioned},
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
				Status: infrav1alpha1.ExoscaleClusterStatus{
					ID:                       &clusterID,
					Initialization:           infrav1alpha1.ExoscaleClusterInitializationStatus{Provisioned: &clusterProvisioned},
					ControlPlaneEndpoint:     &infrav1alpha1.APIEndpointStatus{ID: elasticIPID.String()},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: controlPlaneSecurityGroupID.String()},
					SecurityGroupWorker:      &infrav1alpha1.SecurityGroupStatus{ID: workerSecurityGroupID.String()},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
				Data: map[string][]byte{
					"apikey":    []byte("my-api-key"),
					"apisecret": []byte("my-api-secret"),
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretName, Namespace: ns},
				Data:       map[string][]byte{"value": []byte("#cloud-config")},
			},
			&clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machineName,
					Namespace: ns,
					UID:       types.UID(machineUID.String()),
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:         clusterName,
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: &dataSecretName,
					},
				},
			},
			&infrav1alpha1.ExoscaleMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      exoscaleMachineName,
					Namespace: ns,
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machineName},
					},
				},
				Spec: infrav1alpha1.ExoscaleMachineSpec{
					Template:          template,
					InstanceType:      "standard.small",
					SSHKey:            "ssh-key",
					RootVolumeSizeGiB: &rootVolumeSize,
				},
				Status: infrav1alpha1.ExoscaleMachineStatus{
					Conditions: []metav1.Condition{
						{Type: clusterv1.PausedCondition, Status: metav1.ConditionTrue, Reason: clusterv1.PausedReason},
					},
				},
			},
		).
		Build()

	r := &ExoscaleMachineReconciler{
		Client: client,
		Scheme: scheme,
		NewInstanceService: func(apiKey, apiSecret string, zone egoscale.ZoneName, _ logr.Logger) (domain.InstanceService, error) {
			assert.Equal(t, "my-api-key", apiKey)
			assert.Equal(t, "my-api-secret", apiSecret)
			assert.Equal(t, egoscale.ZoneName("ch-gva-2"), zone)
			return instanceSvc, nil
		},
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	assert.Equal(t, instanceID.String(), updated.Status.InstanceID)
	assert.Equal(t, template, updated.Spec.Template)
	assert.Equal(t, &rootVolumeSize, updated.Spec.RootVolumeSizeGiB)
	assert.Equal(t, []clusterv1.MachineAddress{
		{Type: clusterv1.MachineExternalIP, Address: "1.2.3.4"},
		{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
	}, updated.Status.Addresses)
	assert.True(t, updated.Status.Ready)
	ready := apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.ReadyCondition)
	if assert.NotNil(t, ready) {
		assert.Equal(t, metav1.ConditionTrue, ready.Status)
		assert.Equal(t, clusterv1.ReadyReason, ready.Reason)
	}
	if assert.NotNil(t, updated.Status.Initialization.Provisioned) {
		assert.True(t, *updated.Status.Initialization.Provisioned)
	}
	if assert.NotNil(t, updated.Spec.ProviderID) {
		assert.Equal(t, "exoscale://"+instanceID.String(), *updated.Spec.ProviderID)
	}
	assert.Nil(t, apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.PausedCondition))
}

func TestExoscaleMachineReconciler_Reconcile_prerequisites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		clusterInfrastructureReady bool
		bootstrapDataReady         bool
		machinePaused              bool
		clusterPaused              bool
		wantResult                 reconcile.Result
		conditionType              string
		conditionStatus            metav1.ConditionStatus
		reason                     string
		wantFinalizer              bool
	}{
		{
			name:               "waiting for cluster infrastructure",
			bootstrapDataReady: true,
			wantResult:         reconcile.Result{RequeueAfter: 15 * time.Second},
			conditionType:      clusterv1.ReadyCondition,
			conditionStatus:    metav1.ConditionFalse,
			reason:             clusterv1.WaitingForClusterInfrastructureReadyReason,
			wantFinalizer:      true,
		},
		{
			name:                       "waiting for bootstrap data",
			clusterInfrastructureReady: true,
			conditionType:              clusterv1.ReadyCondition,
			conditionStatus:            metav1.ConditionFalse,
			reason:                     clusterv1.WaitingForBootstrapDataReason,
			wantFinalizer:              true,
		},
		{
			name:                       "ExoscaleMachine paused",
			clusterInfrastructureReady: true,
			bootstrapDataReady:         true,
			machinePaused:              true,
			conditionType:              clusterv1.PausedCondition,
			conditionStatus:            metav1.ConditionTrue,
			reason:                     clusterv1.PausedReason,
		},
		{
			name:                       "Cluster paused",
			clusterInfrastructureReady: true,
			bootstrapDataReady:         true,
			clusterPaused:              true,
			conditionType:              clusterv1.PausedCondition,
			conditionStatus:            metav1.ConditionTrue,
			reason:                     clusterv1.PausedReason,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, r, client, exoscaleMachineName, ns := newMachinePrerequisiteReconciler(
				t,
				tc.clusterInfrastructureReady,
				tc.bootstrapDataReady,
				tc.machinePaused,
				tc.clusterPaused,
			)
			instanceServiceCalled := false
			r.NewInstanceService = func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
				instanceServiceCalled = true
				return mocks.NewInstanceService(t), nil
			}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

			assert.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)
			assert.False(t, instanceServiceCalled)

			updated := &infrav1alpha1.ExoscaleMachine{}
			assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
			condition := apimeta.FindStatusCondition(updated.Status.Conditions, tc.conditionType)
			if assert.NotNil(t, condition) {
				assert.Equal(t, tc.conditionStatus, condition.Status)
				assert.Equal(t, tc.reason, condition.Reason)
			}
			if tc.conditionType == clusterv1.ReadyCondition {
				assert.False(t, updated.Status.Ready)
			}
			if tc.wantFinalizer {
				assert.Contains(t, updated.Finalizers, machineFinalizer)
			} else {
				assert.NotContains(t, updated.Finalizers, machineFinalizer)
			}
			assert.Nil(t, updated.Spec.ProviderID)
			assert.Empty(t, updated.Status.InstanceID)
			assert.Empty(t, updated.Status.Addresses)
			assert.Nil(t, updated.Status.Initialization.Provisioned)
		})
	}
}

func TestExoscaleMachineReconciler_Reconcile_waitsForWorkerSecurityGroup(t *testing.T) {
	t.Parallel()

	ctx, r, client, exoscaleMachineName, ns := newReadyMachineReconciler(t, domain.Instance{}, nil, nil)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, 15*time.Second, result.RequeueAfter)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	assert.Nil(t, updated.Spec.ProviderID)
	assert.Empty(t, updated.Status.InstanceID)
}

func TestExoscaleMachineReconciler_reconcileInstance_controlPlaneEndpoint(t *testing.T) {
	t.Parallel()

	bootstrapSecretName := "bootstrap-data"
	securityGroupID := uuid.New()
	tests := []struct {
		name     string
		endpoint *infrav1alpha1.APIEndpointStatus
		wantErr  string
	}{
		{name: "requires endpoint", wantErr: "control plane Elastic IP is not available"},
		{name: "rejects invalid endpoint ID", endpoint: &infrav1alpha1.APIEndpointStatus{ID: "bad-id"}, wantErr: "invalid control plane Elastic IP ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newExoscaleMachineTestScheme(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretName, Namespace: "default"},
				Data:       map[string][]byte{"value": []byte("#cloud-config")},
			}).Build()
			r := &ExoscaleMachineReconciler{Client: client, Scheme: scheme}
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine",
					Namespace: "default",
					UID:       types.UID(uuid.NewString()),
					Labels:    map[string]string{clusterv1.MachineControlPlaneLabel: ""},
				},
				Spec: clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &bootstrapSecretName}},
			}
			exoMachine := &infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{machineFinalizer}}}
			exoCluster := &infrav1alpha1.ExoscaleCluster{Status: infrav1alpha1.ExoscaleClusterStatus{
				SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: securityGroupID.String()},
				ControlPlaneEndpoint:     tc.endpoint,
			}}

			result, err := r.reconcileInstance(ctx, exoMachine, machine, exoCluster, mocks.NewInstanceService(t))

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Equal(t, reconcile.Result{}, result)
				return
			}
			assert.NoError(t, err)
			ready := apimeta.FindStatusCondition(exoMachine.Status.Conditions, clusterv1.ReadyCondition)
			if assert.NotNil(t, ready) {
				assert.Equal(t, metav1.ConditionFalse, ready.Status)
				assert.Equal(t, clusterv1.NotReadyReason, ready.Reason)
				assert.Equal(t, "Waiting for control plane Elastic IP", ready.Message)
			}
		})
	}
}

func TestExoscaleMachineReconciler_Reconcile_waitsForInstanceRunning(t *testing.T) {
	t.Parallel()

	instanceID := uuid.New()
	ctx, r, client, exoscaleMachineName, ns := newReadyMachineReconciler(t, domain.Instance{ID: instanceID, State: "starting"}, nil, new(uuid.New()))
	exoMachine := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, exoMachine))
	exoMachine.Status.Ready = true
	exoMachine.Status.Initialization.Provisioned = new(true)
	conditions.Set(exoMachine, metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue, Reason: clusterv1.ReadyReason})
	assert.NoError(t, client.Status().Update(ctx, exoMachine))

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, 15*time.Second, result.RequeueAfter)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	assert.Equal(t, instanceID.String(), updated.Status.InstanceID)
	assert.False(t, updated.Status.Ready)
	ready := apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.ReadyCondition)
	if assert.NotNil(t, ready) {
		assert.Equal(t, metav1.ConditionFalse, ready.Status)
		assert.Equal(t, clusterv1.NotReadyReason, ready.Reason)
	}
	if assert.NotNil(t, updated.Status.Initialization.Provisioned) {
		assert.True(t, *updated.Status.Initialization.Provisioned)
	}
}

func TestExoscaleMachineReconciler_Reconcile_returnsInvalidStatusInstanceID(t *testing.T) {
	t.Parallel()

	ctx, r, client, exoscaleMachineName, ns := newReadyMachineReconciler(t, domain.Instance{}, nil, nil)
	exoMachine := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, exoMachine))
	exoMachine.Status.InstanceID = "not-a-uuid"
	assert.NoError(t, client.Status().Update(ctx, exoMachine))

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.Equal(t, reconcile.Result{}, result)
	assert.ErrorContains(t, err, "invalid instanceID in status")
}

func TestExoscaleMachineReconciler_Reconcile_returnsInstanceServiceError(t *testing.T) {
	t.Parallel()

	workerSecurityGroupID := uuid.New()
	ctx, r, client, exoscaleMachineName, ns := newReadyMachineReconciler(t, domain.Instance{}, assert.AnError, &workerSecurityGroupID)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.Equal(t, reconcile.Result{}, result)
	assert.ErrorIs(t, err, assert.AnError)
	assert.ErrorContains(t, err, "upsert instance")
	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	ready := apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.ReadyCondition)
	if assert.NotNil(t, ready) {
		assert.Equal(t, metav1.ConditionFalse, ready.Status)
		assert.Equal(t, clusterv1.InternalErrorReason, ready.Reason)
	}
}
