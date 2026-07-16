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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	)

	ctx := context.Background()
	machineUID := uuid.New()
	instanceID := uuid.New()
	templateID := uuid.New()
	elasticIPID := uuid.New()
	controlPlaneSecurityGroupID := uuid.New()
	nodeSecurityGroupID := uuid.New()
	dataSecretName := bootstrapSecretName
	clusterProvisioned := true

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	instanceSvc := mocks.NewInstanceService(t)
	instanceSvc.EXPECT().UpsertInstance(ctx, machineUID, (*uuid.UUID)(nil), domain.InstanceSpec{
		Name:             machineName,
		Template:         templateID.String(),
		InstanceType:     "standard-2",
		SSHKey:           "ssh-key",
		SecurityGroupIDs: []uuid.UUID{controlPlaneSecurityGroupID},
		ElasticIPID:      &elasticIPID,
		UserData:         "#cloud-config",
	}).Return(domain.Instance{ID: instanceID, State: "running", PublicIP: "1.2.3.4"}, nil)

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
					Initialization:           infrav1alpha1.ExoscaleClusterInitializationStatus{Provisioned: &clusterProvisioned},
					ControlPlaneEndpoint:     &infrav1alpha1.APIEndpointStatus{ID: elasticIPID.String()},
					SecurityGroupControlPlan: &infrav1alpha1.SecurityGroupStatus{ID: controlPlaneSecurityGroupID.String()},
					SecurityGroupNode:        &infrav1alpha1.SecurityGroupStatus{ID: nodeSecurityGroupID.String()},
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
					Template:     templateID.String(),
					InstanceType: "standard-2",
					SSHKey:       "ssh-key",
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
	assert.Equal(t, "running", updated.Status.InstanceState)
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

func TestExoscaleMachineReconciler_Reconcile_ignoresDifferentWatchFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	instanceID := uuid.New()
	deletionTime := metav1.Now()
	scheme := newExoscaleMachineTestScheme(t)
	exoMachine := &infrav1alpha1.ExoscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "filtered-machine",
			Namespace:         "default",
			Labels:            map[string]string{clusterv1.WatchLabel: "other"},
			Finalizers:        []string{machineFinalizer},
			DeletionTimestamp: &deletionTime,
		},
		Status: infrav1alpha1.ExoscaleMachineStatus{InstanceID: instanceID.String()},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&infrav1alpha1.ExoscaleMachine{}).
		WithObjects(exoMachine).
		Build()
	r := &ExoscaleMachineReconciler{Client: client, Scheme: scheme, WatchFilter: "mine"}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: crclient.ObjectKeyFromObject(exoMachine)})

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, crclient.ObjectKeyFromObject(exoMachine), updated))
	assert.Contains(t, updated.Finalizers, machineFinalizer)
}

func TestExoscaleClusterToExoscaleMachines(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newExoscaleMachineTestScheme(t)
	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	exoCluster := &infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{
		Name:      "exo-cluster",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
		}},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, exoCluster).Build()
	want := []reconcile.Request{{NamespacedName: types.NamespacedName{Name: "machine", Namespace: "default"}}}
	mapper := exoscaleClusterToExoscaleMachines(client, func(_ context.Context, obj crclient.Object) []reconcile.Request {
		assert.Equal(t, cluster.Name, obj.GetName())
		return want
	})

	assert.Equal(t, want, mapper(ctx, exoCluster))
}

func TestExoscaleMachineReconciler_Reconcile_waitsForClusterInfrastructure(t *testing.T) {
	t.Parallel()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
	)

	ctx := context.Background()
	machineUID := uuid.New()
	templateID := uuid.New()
	clusterProvisioned := false

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

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
			},
			&clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machineName,
					Namespace: ns,
					UID:       types.UID(machineUID.String()),
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: clusterName,
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
					Template:     templateID.String(),
					InstanceType: "standard-2",
					SSHKey:       "ssh-key",
				},
			},
		).
		Build()

	r := &ExoscaleMachineReconciler{
		Client: client,
		Scheme: scheme,
		NewInstanceService: func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
			assert.Fail(t, "instance service should not be created before cluster infrastructure is ready")
			return nil, nil
		},
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, 15*time.Second, result.RequeueAfter)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	assert.Contains(t, updated.Finalizers, machineFinalizer)
	assert.Empty(t, updated.Status.InstanceID)
}

func TestExoscaleMachineReconciler_Reconcile_waitsForBootstrapData(t *testing.T) {
	t.Parallel()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
	)

	ctx := context.Background()
	machineUID := uuid.New()
	templateID := uuid.New()
	clusterProvisioned := true

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

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
			},
			&clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machineName,
					Namespace: ns,
					UID:       types.UID(machineUID.String()),
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: clusterName,
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
					Template:     templateID.String(),
					InstanceType: "standard-2",
					SSHKey:       "ssh-key",
				},
			},
		).
		Build()

	r := &ExoscaleMachineReconciler{
		Client: client,
		Scheme: scheme,
		NewInstanceService: func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
			assert.Fail(t, "instance service should not be created before bootstrap data is ready")
			return nil, nil
		},
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	assert.Contains(t, updated.Finalizers, machineFinalizer)
	assert.Nil(t, updated.Spec.ProviderID)
	assert.Empty(t, updated.Status.InstanceID)
}

func TestExoscaleMachineReconciler_Reconcile_setsPausedCondition(t *testing.T) {
	t.Parallel()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
	)

	ctx := context.Background()
	templateID := uuid.New()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

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
			},
			&infrav1alpha1.ExoscaleMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      exoscaleMachineName,
					Namespace: ns,
					Labels:    map[string]string{clusterv1.ClusterNameLabel: clusterName},
					Annotations: map[string]string{
						clusterv1.PausedAnnotation: "",
					},
				},
				Spec: infrav1alpha1.ExoscaleMachineSpec{
					Template:     templateID.String(),
					InstanceType: "standard-2",
					SSHKey:       "ssh-key",
				},
			},
		).
		Build()

	r := &ExoscaleMachineReconciler{
		Client: client,
		Scheme: scheme,
		NewInstanceService: func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
			assert.Fail(t, "instance service should not be created while paused")
			return nil, nil
		},
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}})

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	updated := &infrav1alpha1.ExoscaleMachine{}
	assert.NoError(t, client.Get(ctx, types.NamespacedName{Name: exoscaleMachineName, Namespace: ns}, updated))
	paused := apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.PausedCondition)
	if assert.NotNil(t, paused) {
		assert.Equal(t, metav1.ConditionTrue, paused.Status)
		assert.Equal(t, clusterv1.PausedReason, paused.Reason)
	}
}

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
	machineUID := uuid.New()
	instanceID := uuid.New()

	tests := []struct {
		name             string
		ownerReference   bool
		ownerMachine     bool
		statusInstanceID string
		deleteErr        error
		wantService      bool
		wantErr          error
		wantFinalizer    bool
	}{
		{name: "recovers instance by Machine UID", ownerReference: true, ownerMachine: true, wantService: true},
		{name: "recovers instance when owner Machine is gone", ownerReference: true, wantService: true},
		{name: "status instance without owner keeps finalizer", statusInstanceID: instanceID.String(), deleteErr: assert.AnError, wantService: true, wantErr: assert.AnError, wantFinalizer: true},
		{name: "without either identifier removes finalizer"},
		{name: "instance not found removes finalizer", ownerReference: true, statusInstanceID: instanceID.String(), deleteErr: domain.ErrInstanceNotFound, wantService: true},
		{name: "delete error keeps finalizer", ownerReference: true, statusInstanceID: instanceID.String(), deleteErr: assert.AnError, wantService: true, wantErr: assert.AnError, wantFinalizer: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := newExoscaleMachineTestScheme(t)
			instanceSvc := mocks.NewInstanceService(t)
			var expectedMachineID, expectedInstanceID *uuid.UUID
			if tc.ownerReference {
				expectedMachineID = &machineUID
			}
			if tc.statusInstanceID != "" {
				expectedInstanceID = &instanceID
			}
			if tc.wantService {
				instanceSvc.EXPECT().DeleteInstance(ctx, expectedMachineID, expectedInstanceID).Return(tc.deleteErr)
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
						UID:       types.UID(machineUID.String()),
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
					UID:        types.UID(machineUID.String()),
				}}
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
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}

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
		})
	}
}

func TestExoscaleMachineReconciler_Reconcile_waitsForNodeSecurityGroup(t *testing.T) {
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
	assert.Equal(t, "starting", updated.Status.InstanceState)
	assert.True(t, updated.Status.Ready)
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

	nodeSecurityGroupID := uuid.New()
	ctx, r, client, exoscaleMachineName, ns := newReadyMachineReconciler(t, domain.Instance{}, assert.AnError, &nodeSecurityGroupID)

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

func newExoscaleMachineTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, clusterv1.AddToScheme(scheme))
	assert.NoError(t, infrav1alpha1.AddToScheme(scheme))
	return scheme
}

func newReadyMachineReconciler(t *testing.T, instance domain.Instance, upsertErr error, nodeSecurityGroupID *uuid.UUID) (context.Context, *ExoscaleMachineReconciler, crclient.Client, string, string) {
	t.Helper()

	const (
		ns                  = "default"
		clusterName         = "test-cluster"
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
		secretName          = "exoscale-creds"
		bootstrapSecretName = "bootstrap-data"
	)

	ctx := context.Background()
	machineUID := uuid.New()
	templateID := uuid.New()
	dataSecretName := bootstrapSecretName
	clusterProvisioned := true
	scheme := newExoscaleMachineTestScheme(t)
	instanceSvc := mocks.NewInstanceService(t)

	var securityGroupNode *infrav1alpha1.SecurityGroupStatus
	if nodeSecurityGroupID != nil {
		securityGroupNode = &infrav1alpha1.SecurityGroupStatus{ID: nodeSecurityGroupID.String()}
		instanceSvc.EXPECT().UpsertInstance(ctx, machineUID, (*uuid.UUID)(nil), domain.InstanceSpec{
			Name:             machineName,
			Template:         templateID.String(),
			InstanceType:     "standard-2",
			SSHKey:           "ssh-key",
			SecurityGroupIDs: []uuid.UUID{*nodeSecurityGroupID},
			UserData:         "#cloud-config",
		}).Return(instance, upsertErr)
	}

	clientBuilder := fake.NewClientBuilder().
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
					Initialization:    infrav1alpha1.ExoscaleClusterInitializationStatus{Provisioned: &clusterProvisioned},
					SecurityGroupNode: securityGroupNode,
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
						clusterv1.ClusterNameLabel: clusterName,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
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
					Template:     templateID.String(),
					InstanceType: "standard-2",
					SSHKey:       "ssh-key",
				},
			},
		)

	client := clientBuilder.Build()
	r := &ExoscaleMachineReconciler{
		Client: client,
		Scheme: scheme,
		NewInstanceService: func(string, string, egoscale.ZoneName, logr.Logger) (domain.InstanceService, error) {
			return instanceSvc, nil
		},
	}

	return ctx, r, client, exoscaleMachineName, ns
}
