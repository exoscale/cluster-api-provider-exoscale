package controller

import (
	"context"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
