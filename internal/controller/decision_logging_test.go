package controller

import (
	"context"
	"strings"
	"testing"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/go-logr/logr/funcr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestClusterDecisionLogging(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		verbosity int
		wantLog   bool
	}{
		{name: "hidden below V(2)", verbosity: 1},
		{name: "present at V(2)", verbosity: 2, wantLog: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, output := captureDecisionLogs(context.Background(), tc.verbosity)
			scheme := runtime.NewScheme()
			assert.NoError(t, clusterv1.AddToScheme(scheme))
			assert.NoError(t, infrav1alpha1.AddToScheme(scheme))

			const name = "cluster"
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&infrav1alpha1.ExoscaleCluster{}).
				WithObjects(
					&clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{
						Name: name, Annotations: map[string]string{clusterv1.ManagedByAnnotation: ""},
					}},
					&infrav1alpha1.ExoscaleCluster{ObjectMeta: metav1.ObjectMeta{
						Name: name,
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: clusterv1.GroupVersion.String(), Kind: "Cluster", Name: name,
						}},
					}},
				).
				Build()

			_, err := (&ExoscaleClusterReconciler{Client: client, Scheme: scheme}).Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			assert.NoError(t, err)

			logOutput := output.String()
			if tc.wantLog {
				assert.Equal(t, 1, strings.Count(logOutput, `"msg":"Cluster decision"`))
				assert.Contains(t, logOutput, `"action":"skip-externally-managed"`)
			} else {
				assert.NotContains(t, logOutput, `"msg":"Cluster decision"`)
			}
		})
	}
}

func TestMachineDecisionLogging(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		verbosity int
		wantLog   bool
	}{
		{name: "hidden below V(2)", verbosity: 1},
		{name: "present at V(2)", verbosity: 2, wantLog: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			securityGroupID := uuid.New()
			ctx, r, _, exoscaleMachineName, namespace := newReadyMachineReconciler(
				t,
				domain.Instance{ID: uuid.New(), State: "running"},
				nil,
				&securityGroupID,
				nil,
			)
			ctx, output := captureDecisionLogs(ctx, tc.verbosity)

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
				Name: exoscaleMachineName, Namespace: namespace,
			}})
			assert.NoError(t, err)

			logOutput := output.String()
			if tc.wantLog {
				assert.Equal(t, 2, strings.Count(logOutput, `"msg":"Machine decision"`))
				assert.Contains(t, logOutput, `"msg":"Machine decision","action":"upsert"`)
				assert.Contains(t, logOutput, `"msg":"Machine decision","action":"ready"`)
			} else {
				assert.NotContains(t, logOutput, `"msg":"Machine decision"`)
			}
		})
	}
}

func captureDecisionLogs(ctx context.Context, verbosity int) (context.Context, *strings.Builder) {
	var output strings.Builder
	logger := funcr.NewJSON(func(entry string) {
		output.WriteString(entry)
		output.WriteByte('\n')
	}, funcr.Options{Verbosity: verbosity})
	return logf.IntoContext(ctx, logger), &output
}
