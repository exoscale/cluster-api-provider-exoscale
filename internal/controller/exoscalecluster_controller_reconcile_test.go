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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_ExoscaleClusterReconciler_Reconcile_nominal(t *testing.T) {
	t.Parallel()

	const (
		ns          = "default"
		clusterName = "test-cluster"
		secretName  = "exoscale-creds"
	)

	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	tests := []struct {
		name           string
		k8sClient      func(b *fake.ClientBuilder)
		clusterService func(m *mocks.ClusterService)
		check          func(t *testing.T, c client.Client)
		err            error
		output         reconcile.Result
	}{
		{
			name: "reconcile succeeds",
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
					ReconcileCluster(mock.Anything, mock.Anything).
					RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
						return cluster, nil
					})
			},
			check: func(t *testing.T, c client.Client) {
				t.Helper()
				updated := &infrav1alpha1.ExoscaleCluster{}
				assert.NoError(t, c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: ns}, updated))

				assert.Contains(t, updated.Finalizers, infrav1alpha1.ExoscaleClusterFinalizer)
				assert.NotEmpty(t, updated.Spec.ClusterID)

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

				assert.NotEmpty(t, updated.Spec.ClusterID)
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

				assert.NotEmpty(t, updated.Spec.ClusterID)
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

				assert.NotEmpty(t, updated.Spec.ClusterID)

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

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, result)

			if ut.check != nil {
				ut.check(t, r.Client)
			}
		})
	}
}

func Test_ExoscaleClusterReconciler_Reconcile_error(t *testing.T) {
	t.Parallel()

	const (
		ns          = "default"
		clusterName = "test-cluster"
		secretName  = "exoscale-creds"
	)

	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)

	tests := []struct {
		name           string
		k8sClient      func(b *fake.ClientBuilder)
		clusterService func(m *mocks.ClusterService)
		check          func(t *testing.T, c client.Client)
		err            error
		output         reconcile.Result
	}{
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
					RunAndReturn(func(_ context.Context, cluster infrav1alpha1.ExoscaleCluster) (infrav1alpha1.ExoscaleCluster, error) {
						return cluster, assert.AnError
					})
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
		{
			name: "deferred patch returns error",
			k8sClient: func(b *fake.ClientBuilder) {
				// A paused cluster returns no reconciliation error, but setting its ID and Paused
				// condition still makes the deferred patch run.
				b.WithObjects(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
					},
					&infrav1alpha1.ExoscaleCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:        clusterName,
							Namespace:   ns,
							Annotations: map[string]string{clusterv1.PausedAnnotation: ""},
							OwnerReferences: []metav1.OwnerReference{{
								APIVersion: clusterv1.GroupVersion.String(),
								Kind:       "Cluster",
								Name:       clusterName,
							}},
						},
					},
				).WithInterceptorFuncs(interceptor.Funcs{
					// Force the deferred status patch to fail so the test verifies that
					// Reconcile returns it.
					SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
						return assert.AnError
					},
				})
			},
			err:    assert.AnError,
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

			assert.ErrorIs(t, err, ut.err)
			assert.Equal(t, ut.output, result)

			if ut.check != nil {
				ut.check(t, r.Client)
			}
		})
	}
}
