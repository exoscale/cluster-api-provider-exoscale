package controller

import (
	"context"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
