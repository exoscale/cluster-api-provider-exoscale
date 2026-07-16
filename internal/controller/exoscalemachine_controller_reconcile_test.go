package controller

import (
	"context"
	"testing"
	"time"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
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
	nodeSecurityGroupID := uuid.New()
	dataSecretName := bootstrapSecretName
	clusterProvisioned := true

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	instanceSvc := instanceServiceStub{
		upsert: func(_ context.Context, gotMachineID uuid.UUID, gotInstanceID *uuid.UUID, spec domain.InstanceSpec) (domain.Instance, error) {
			assert.Equal(t, machineUID, gotMachineID)
			assert.Nil(t, gotInstanceID)
			assert.Equal(t, machineName, spec.Name)
			assert.Equal(t, templateID, spec.TemplateID)
			assert.Equal(t, "standard-2", spec.InstanceType)
			assert.Equal(t, "ssh-key", spec.SSHKey)
			assert.Equal(t, "#cloud-config", spec.UserData)
			assert.Equal(t, []uuid.UUID{nodeSecurityGroupID}, spec.SecurityGroupIDs)

			return domain.Instance{ID: instanceID, State: "running", PublicIP: "1.2.3.4"}, nil
		},
	}

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
					SecurityGroupNode: &infrav1alpha1.SecurityGroupStatus{ID: nodeSecurityGroupID.String()},
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
					TemplateID:   templateID.String(),
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
	if assert.NotNil(t, updated.Status.Initialization.Provisioned) {
		assert.True(t, *updated.Status.Initialization.Provisioned)
	}
	if assert.NotNil(t, updated.Spec.ProviderID) {
		assert.Equal(t, "exoscale://"+instanceID.String(), *updated.Spec.ProviderID)
	}
	assert.Nil(t, apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.PausedCondition))
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
			},
			&infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: exoscaleClusterName, Namespace: ns},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					Initialization: infrav1alpha1.ExoscaleClusterInitializationStatus{Provisioned: &clusterProvisioned},
				},
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
					TemplateID:   templateID.String(),
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
			},
			&infrav1alpha1.ExoscaleCluster{
				ObjectMeta: metav1.ObjectMeta{Name: exoscaleClusterName, Namespace: ns},
				Status: infrav1alpha1.ExoscaleClusterStatus{
					Initialization: infrav1alpha1.ExoscaleClusterInitializationStatus{Provisioned: &clusterProvisioned},
				},
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
					TemplateID:   templateID.String(),
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
		machineName         = "test-machine"
		exoscaleClusterName = "test-exoscale-cluster"
		exoscaleMachineName = "test-exoscale-machine"
	)

	ctx := context.Background()
	machineUID := uuid.New()
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
					Annotations: map[string]string{
						clusterv1.PausedAnnotation: "",
					},
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machineName},
					},
				},
				Spec: infrav1alpha1.ExoscaleMachineSpec{
					TemplateID:   templateID.String(),
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

type instanceServiceStub struct {
	upsert func(context.Context, uuid.UUID, *uuid.UUID, domain.InstanceSpec) (domain.Instance, error)
}

func (s instanceServiceStub) UpsertInstance(ctx context.Context, machineID uuid.UUID, instanceID *uuid.UUID, spec domain.InstanceSpec) (domain.Instance, error) {
	return s.upsert(ctx, machineID, instanceID, spec)
}

func (s instanceServiceStub) DeleteInstance(context.Context, uuid.UUID) error { return nil }
