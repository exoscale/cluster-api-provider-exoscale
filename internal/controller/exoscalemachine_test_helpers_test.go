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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
