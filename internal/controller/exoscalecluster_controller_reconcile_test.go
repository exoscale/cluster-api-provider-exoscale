package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestExoscaleClusterReconciler_Reconcile_nominal(t *testing.T) {
	t.Parallel()

	const (
		ns             = "default"
		clusterName    = "test-cluster"
		secretName     = "exoscale-creds"
		otherFinalizer = "example.com/other"
	)

	ctx := context.Background()
	firstPatchUsesOptimisticLock := false

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	tests := []struct {
		name           string
		k8sClient      func(b *fake.ClientBuilder)
		clusterService func(m *mocks.ClusterService)
		beforeService  func(t *testing.T, c client.Client)
		check          func(t *testing.T, c client.Client)
		err            error
		output         reconcile.Result
	}{
		{
			name: "reconcile succeeds",
			k8sClient: func(b *fake.ClientBuilder) {
				firstPatch := true
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
						},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: ns,
						},
						Data: map[string][]byte{
							"apikey":    []byte("my-api-key"),
							"apisecret": []byte("my-api-secret"),
						},
					},
				).WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						if _, ok := obj.(*infrav1alpha1.ExoscaleCluster); ok && firstPatch {
							data, err := patch.Data(obj)
							assert.NoError(t, err)
							firstPatchUsesOptimisticLock = strings.Contains(string(data), `"resourceVersion"`)
							firstPatch = false
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
				})
			},
			clusterService: func(m *mocks.ClusterService) {
				m.EXPECT().
					ReconcileCluster(mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
						return cluster, nil
					})
			},
			beforeService: func(t *testing.T, c client.Client) {
				t.Helper()
				stored := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, stored))
				assert.Contains(t, stored.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
				assert.NotNil(t, stored.Status.ID)
				assert.Equal(t, *stored.Status.ID, stored.Annotations[domain.ClusterIDKey])

				before := stored.DeepCopy()
				stored.Finalizers = append(stored.Finalizers, otherFinalizer)
				assert.NoError(t, c.Patch(ctx, stored, client.MergeFrom(before)))
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))

				assert.Contains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
				assert.Contains(t, updated.Finalizers, otherFinalizer)
				assert.True(t, firstPatchUsesOptimisticLock)
				assert.NotNil(t, updated.Status.ID)
				assert.Equal(t, *updated.Status.ID, updated.Annotations[domain.ClusterIDKey])

				ready := apimeta.FindStatusCondition(updated.Status.Conditions, infrav1alpha1.ReadyCondition)
				if assert.NotNil(t, ready) {
					assert.Equal(t, metav1.ConditionTrue, ready.Status)
					assert.Equal(t, infrav1alpha1.ReconcileSuccessReason, ready.Reason)
				}
			},
			output: reconcile.Result{},
		},
		{
			name: "no capi cluster",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
				)
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))

				assert.NotNil(t, updated.Status.ID)
				assert.NotContains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
			},
			output: reconcile.Result{},
		},
		{
			name: "externally managed",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							Annotations: map[string]string{
								clusterv1.ManagedByAnnotation: "",
							},
						},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
				)
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))

				assert.NotNil(t, updated.Status.ID)
				assert.NotContains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
			},
			output: reconcile.Result{},
		},
		{
			name: "paused",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
						},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							Annotations: map[string]string{
								clusterv1.PausedAnnotation: "",
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
				)
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))

				assert.NotNil(t, updated.Status.ID)
				assert.NotContains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)

				paused := apimeta.FindStatusCondition(updated.Status.Conditions, clusterv1.PausedCondition)
				if assert.NotNil(t, paused) {
					assert.Equal(t, metav1.ConditionTrue, paused.Status)
					assert.Equal(t, clusterv1.PausedReason, paused.Reason)
				}
			},
			output: reconcile.Result{},
		},
		{
			name: "reconcile succeeds - delete",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
						},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
							DeletionTimestamp: func() *metav1.Time { v := metav1.NewTime(time.Now()); return &v }(),
							Finalizers:        []string{infrav1alpha1.ExoscaleClusterFinalizer},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: ns,
						},
						Data: map[string][]byte{
							"apikey":    []byte("my-api-key"),
							"apisecret": []byte("my-api-secret"),
						},
					},
				)
			},
			clusterService: func(m *mocks.ClusterService) {
				m.EXPECT().
					DeleteCluster(mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
						return cluster, nil
					})
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated)
				assert.True(t, apierrors.IsNotFound(err), "Should be a not found error, got: %s", err.Error())
			},
			output: reconcile.Result{},
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&infrav1alpha1.ExoscaleCluster{})
			if ut.k8sClient != nil {
				ut.k8sClient(builder)
			}

			svc := mocks.NewClusterService(t)
			if ut.clusterService != nil {
				ut.clusterService(svc)
			}

			k8sClient := builder.Build()
			r := &ExoscaleClusterReconciler{
				Client: k8sClient,
				Scheme: scheme,
				NewClusterService: func(apikey, apiSecret string, _ egoscale.ZoneName, _ logr.Logger) (domain.ClusterService, error) {
					assert.Equal(t, apikey, "my-api-key")
					assert.Equal(t, apiSecret, "my-api-secret")
					if ut.beforeService != nil {
						ut.beforeService(t, k8sClient)
					}
					return svc, nil
				},
			}

			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: clusterName, Namespace: ns},
			})

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, result)

			if ut.check != nil {
				ut.check(t, r.Client)
			}
		})
	}
}

func TestEnsureClusterID(t *testing.T) {
	t.Parallel()

	t.Run("creates and persists a new ID", func(t *testing.T) {
		cluster := &infrav1alpha1.ExoscaleCluster{}

		changed, err := ensureClusterID(cluster)

		assert.NoError(t, err)
		assert.True(t, changed)
		assert.NotNil(t, cluster.Status.ID)
		assert.Equal(t, *cluster.Status.ID, cluster.Annotations[domain.ClusterIDKey])
	})

	t.Run("restores status from the persisted ID after move", func(t *testing.T) {
		clusterID := uuid.NewString()
		cluster := &infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{domain.ClusterIDKey: clusterID},
		}}

		changed, err := ensureClusterID(cluster)

		assert.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, clusterID, *cluster.Status.ID)
	})

	t.Run("backfills the annotation for existing clusters", func(t *testing.T) {
		clusterID := uuid.NewString()
		cluster := &infrav1alpha1.ExoscaleCluster{Status: infrav1alpha1.ExoscaleClusterStatus{ID: &clusterID}}

		changed, err := ensureClusterID(cluster)

		assert.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, clusterID, cluster.Annotations[domain.ClusterIDKey])
	})

	t.Run("rejects conflicting identities", func(t *testing.T) {
		statusID := uuid.NewString()
		cluster := &infrav1alpha1.ExoscaleCluster{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.ClusterIDKey: uuid.NewString()}},
			Status:     infrav1alpha1.ExoscaleClusterStatus{ID: &statusID},
		}

		_, err := ensureClusterID(cluster)

		assert.ErrorContains(t, err, "does not match")
	})
}

func TestExoscaleClusterFinalizerPatch(t *testing.T) {
	t.Parallel()

	const (
		ns             = "default"
		clusterName    = "test-cluster"
		secretName     = "exoscale-creds"
		otherFinalizer = "example.com/other"
	)

	tests := []struct {
		name      string
		patchErr  error
		wantError bool
	}{
		{name: "removes only the CAPX finalizer"},
		{
			name:     "ignores NotFound",
			patchErr: apierrors.NewNotFound(schema.GroupResource{Group: infrav1alpha1.GroupVersion.Group, Resource: "exoscaleclusters"}, clusterName),
		},
		{
			name: "returns conflict",
			patchErr: apierrors.NewConflict(
				schema.GroupResource{Group: infrav1alpha1.GroupVersion.Group, Resource: "exoscaleclusters"},
				clusterName,
				assert.AnError,
			),
			wantError: true,
		},
		{name: "returns generic error", patchErr: assert.AnError, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			assert.NoError(t, clientgoscheme.AddToScheme(scheme))
			assert.NoError(t, clusterv1.AddToScheme(scheme))
			assert.NoError(t, infrav1alpha1.AddToScheme(scheme))

			clusterID := uuid.NewString()
			deletionTimestamp := metav1.NewTime(time.Now())
			patchCalls := 0
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&infrav1alpha1.ExoscaleCluster{}).
				WithObjects(
					&clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns}},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:              clusterName,
							Namespace:         ns,
							DeletionTimestamp: &deletionTimestamp,
							Finalizers:        []string{infrav1alpha1.ExoscaleClusterFinalizer, otherFinalizer},
							Annotations:       map[string]string{domain.ClusterIDKey: clusterID},
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name: secretName, ApiKey: "apikey", APISecret: "apisecret",
							},
						},
						Status: infrav1alpha1.ExoscaleClusterStatus{ID: &clusterID},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
						Data: map[string][]byte{
							"apikey": []byte("my-api-key"), "apisecret": []byte("my-api-secret"),
						},
					},
				).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						if _, ok := obj.(*infrav1alpha1.ExoscaleCluster); ok {
							patchCalls++
							if tt.patchErr != nil {
								return tt.patchErr
							}
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
				}).
				Build()

			svc := mocks.NewClusterService(t)
			svc.EXPECT().DeleteCluster(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
					return cluster, nil
				})
			r := &ExoscaleClusterReconciler{
				Client: k8sClient,
				Scheme: scheme,
				NewClusterService: func(_, _ string, _ egoscale.ZoneName, _ logr.Logger) (domain.ClusterService, error) {
					return svc, nil
				},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: clusterName, Namespace: ns}})

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, 1, patchCalls)
			if tt.patchErr == nil {
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))
				assert.NotContains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
				assert.Contains(t, updated.Finalizers, otherFinalizer)
			}
		})
	}
}

func TestExoscaleClusterReconciler_Reconcile_error(t *testing.T) {
	t.Parallel()

	const (
		ns             = "default"
		clusterName    = "test-cluster"
		secretName     = "exoscale-creds"
		otherFinalizer = "example.com/other"
	)

	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)
	earlyPatchCalls := 0

	tests := []struct {
		name           string
		k8sClient      func(b *fake.ClientBuilder)
		clusterService func(m *mocks.ClusterService)
		check          func(t *testing.T, c client.Client)
		checkErr       func(t *testing.T, err error)
		err            error
		output         reconcile.Result
	}{
		{
			name: "finalizer patch conflicts with concurrent metadata update",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns}},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name: secretName, ApiKey: "apikey", APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
						Data: map[string][]byte{
							"apikey": []byte("my-api-key"), "apisecret": []byte("my-api-secret"),
						},
					},
				).WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						if _, ok := obj.(*infrav1alpha1.ExoscaleCluster); ok {
							earlyPatchCalls++
							if earlyPatchCalls == 1 {
								latest := &infrav1alpha1.ExoscaleCluster{}
								assert.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), latest))
								before := latest.DeepCopy()
								latest.Finalizers = append(latest.Finalizers, otherFinalizer)
								assert.NoError(t, c.Patch(ctx, latest, client.MergeFrom(before)))
							}
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
				})
			},
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assert.True(t, apierrors.IsConflict(err), "expected conflict, got %v", err)
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				assert.Equal(t, 1, earlyPatchCalls)
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))
				assert.Equal(t, []string{otherFinalizer}, updated.Finalizers)
			},
		},
		{
			name: "get ExoscaleCluster returns error",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*infrav1alpha1.ExoscaleCluster); ok {
							return assert.AnError
						}
						return c.Get(ctx, key, obj, opts...)
					},
				})
			},
			err:    assert.AnError,
			output: reconcile.Result{},
		},
		{
			name: "get secret returns error",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
				).WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1.Secret); ok {
							return assert.AnError
						}
						return c.Get(ctx, key, obj, opts...)
					},
				})
			},
			err:    assert.AnError,
			output: reconcile.Result{},
		},
		{
			name: "getAPICreds returns error",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
						Data:       map[string][]byte{},
					},
				)
			},
			err:    errInvalidCreds,
			output: reconcile.Result{},
		},
		{
			name: "reconcileCluster returns error",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
						Data: map[string][]byte{
							"apikey":    []byte("my-api-key"),
							"apisecret": []byte("my-api-secret"),
						},
					},
				)
			},
			clusterService: func(m *mocks.ClusterService) {
				m.EXPECT().
					ReconcileCluster(mock.Anything, mock.Anything).
					Return(infrav1alpha1.ExoscaleCluster{}, assert.AnError)
			},
			err:    assert.AnError,
			output: reconcile.Result{},
		},
		{
			name: "deleteCluster cluster returned an error",
			k8sClient: func(b *fake.ClientBuilder) {
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
						},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterName,
							Namespace: ns,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       clusterName,
								},
							},
							DeletionTimestamp: func() *metav1.Time { v := metav1.NewTime(time.Now()); return &v }(),
							Finalizers:        []string{infrav1alpha1.ExoscaleClusterFinalizer},
						},
						Spec: infrav1alpha1.ExoscaleClusterSpec{
							Zone: "ch-gva-2",
							ControlPlaneEndpoint: infrav1alpha1.APIEndpoint{
								Port: 6443,
							},
							ExoscaleSecret: infrav1alpha1.ExoscaleSecretRef{
								Name:      secretName,
								ApiKey:    "apikey",
								APISecret: "apisecret",
							},
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: ns,
						},
						Data: map[string][]byte{
							"apikey":    []byte("my-api-key"),
							"apisecret": []byte("my-api-secret"),
						},
					},
				)
			},
			clusterService: func(m *mocks.ClusterService) {
				m.EXPECT().
					DeleteCluster(mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
						return cluster, assert.AnError
					})
			},
			output: reconcile.Result{},
			err:    assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&infrav1alpha1.ExoscaleCluster{})
			if ut.k8sClient != nil {
				ut.k8sClient(builder)
			}

			svc := mocks.NewClusterService(t)
			if ut.clusterService != nil {
				ut.clusterService(svc)
			}

			r := &ExoscaleClusterReconciler{
				Client: builder.Build(),
				Scheme: scheme,
				NewClusterService: func(apikey, apiSecret string, _ egoscale.ZoneName, _ logr.Logger) (domain.ClusterService, error) {
					assert.Equal(t, apikey, "my-api-key")
					assert.Equal(t, apiSecret, "my-api-secret")
					return svc, nil
				},
			}

			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: clusterName, Namespace: ns},
			})

			if ut.checkErr != nil {
				ut.checkErr(t, err)
			} else {
				assert.ErrorIs(t, err, ut.err)
			}
			assert.Equal(t, ut.output, result)

			if ut.check != nil {
				ut.check(t, r.Client)
			}
		})
	}
}
