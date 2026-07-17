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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	calls := 0
	mapper := exoscaleClusterToExoscaleMachines(client, func(_ context.Context, obj crclient.Object) []reconcile.Request {
		calls++
		assert.Equal(t, cluster.Name, obj.GetName())
		return want
	})

	assert.Equal(t, want, mapper(ctx, exoCluster))
	assert.Nil(t, mapper(ctx, &corev1.Secret{}))
	assert.Nil(t, mapper(ctx, &infrav1alpha1.ExoscaleCluster{}))
	assert.Nil(t, mapper(ctx, &infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{
		Name:      "missing-owner",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       "missing",
		}},
	}}))
	assert.Equal(t, 1, calls)
}

func TestExoscaleMachineReconciler_SetupWithManager(t *testing.T) {
	t.Parallel()

	t.Run("rejects scheme without ExoscaleMachine", func(t *testing.T) {
		scheme := runtime.NewScheme()
		mgr := &exoscaleMachineTestManager{scheme: scheme}

		err := (&ExoscaleMachineReconciler{}).SetupWithManager(mgr)

		assert.Error(t, err)
		assert.Zero(t, mgr.added)
	})

	t.Run("registers controller", func(t *testing.T) {
		scheme := newExoscaleMachineTestScheme(t)
		mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{infrav1alpha1.GroupVersion})
		mapper.Add(infrav1alpha1.GroupVersion.WithKind("ExoscaleMachine"), apimeta.RESTScopeNamespace)
		mapper.Add(infrav1alpha1.GroupVersion.WithKind("ExoscaleMachineList"), apimeta.RESTScopeNamespace)
		client := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).Build()
		mgr := &exoscaleMachineTestManager{
			scheme: scheme,
			client: client,
			cache:  &informertest.FakeInformers{Scheme: scheme},
		}

		err := (&ExoscaleMachineReconciler{Client: client, Scheme: scheme}).SetupWithManager(mgr)

		assert.NoError(t, err)
		assert.Equal(t, 1, mgr.added)
	})
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

func Test_deletionInstanceIDs_rejectsMalformedIdentifiers(t *testing.T) {
	t.Parallel()

	instanceID := uuid.New()
	machineID := uuid.New()
	tests := []struct {
		name           string
		machine        infrav1alpha1.ExoscaleMachine
		wantMachineID  *uuid.UUID
		wantInstanceID *uuid.UUID
		wantErr        string
	}{
		{
			name:    "invalid status instance ID",
			machine: infrav1alpha1.ExoscaleMachine{Status: infrav1alpha1.ExoscaleMachineStatus{InstanceID: "bad-id"}},
			wantErr: "invalid instanceID in status",
		},
		{
			name: "invalid owner API version",
			machine: infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "cluster.x-k8s.io/v1beta2/extra", Kind: "Machine", UID: types.UID(machineID.String()),
			}}}},
			wantErr: "invalid owner API version",
		},
		{
			name: "invalid Machine owner UID",
			machine: infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", UID: "bad-id",
			}}}},
			wantErr: "invalid Machine owner UID",
		},
		{
			name: "ignores malformed non-Machine owner",
			machine: infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "bad/version/extra", Kind: "Other", UID: "bad-id",
			}}}},
		},
		{
			name: "ignores foreign Machine owner",
			machine: infrav1alpha1.ExoscaleMachine{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "other.example.io/v1", Kind: "Machine", UID: "bad-id",
				}}},
				Status: infrav1alpha1.ExoscaleMachineStatus{InstanceID: instanceID.String()},
			},
			wantInstanceID: &instanceID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMachineID, gotInstanceID, err := deletionInstanceIDs(&tc.machine)

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantMachineID, gotMachineID)
			assert.Equal(t, tc.wantInstanceID, gotInstanceID)
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

func TestExoscaleMachineReconciler_reconcileNormal_controlPlaneEndpoint(t *testing.T) {
	t.Parallel()

	bootstrapSecretName := "bootstrap-data"
	securityGroupID := uuid.New()
	tests := []struct {
		name     string
		endpoint *infrav1alpha1.APIEndpointStatus
		wantWait bool
		wantErr  string
	}{
		{name: "waits for endpoint", wantWait: true},
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

			result, err := r.reconcileNormal(ctx, exoMachine, machine, exoCluster, mocks.NewInstanceService(t), nil)

			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Equal(t, reconcile.Result{}, result)
				return
			}
			assert.NoError(t, err)
			if tc.wantWait {
				assert.Equal(t, 15*time.Second, result.RequeueAfter)
			}
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
	assert.Equal(t, "starting", updated.Status.InstanceState)
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

type exoscaleMachineTestManager struct {
	ctrl.Manager
	scheme *runtime.Scheme
	client crclient.Client
	cache  crcache.Cache
	added  int
}

func (m *exoscaleMachineTestManager) Add(_ manager.Runnable) error {
	m.added++
	return nil
}

func (m *exoscaleMachineTestManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

func (m *exoscaleMachineTestManager) GetClient() crclient.Client {
	return m.client
}

func (m *exoscaleMachineTestManager) GetCache() crcache.Cache {
	return m.cache
}

func (m *exoscaleMachineTestManager) GetLogger() logr.Logger {
	return logr.Discard()
}

func (m *exoscaleMachineTestManager) GetControllerOptions() crconfig.Controller {
	return crconfig.Controller{}
}
