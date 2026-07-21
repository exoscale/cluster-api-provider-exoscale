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

func TestFilterExoscaleMachineRequests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := newExoscaleMachineTestScheme(t)
	matching := &infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{
		Name:      "matching",
		Namespace: "default",
		Labels:    map[string]string{clusterv1.WatchLabel: "blue"},
	}}
	mismatched := &infrav1alpha1.ExoscaleMachine{ObjectMeta: metav1.ObjectMeta{
		Name:      "mismatched",
		Namespace: "default",
		Labels:    map[string]string{clusterv1.WatchLabel: "green"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(matching, mismatched).Build()
	requests := []reconcile.Request{
		{NamespacedName: crclient.ObjectKeyFromObject(matching)},
		{NamespacedName: crclient.ObjectKeyFromObject(mismatched)},
		{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}},
	}
	mapper := func(context.Context, crclient.Object) []reconcile.Request { return requests }

	filtered := filterExoscaleMachineRequests(client, "blue", mapper)

	assert.Equal(t, requests[:1], filtered(ctx, &clusterv1.Cluster{}))
	assert.Equal(t, requests, filterExoscaleMachineRequests(client, "", mapper)(ctx, &clusterv1.Cluster{}))
}
