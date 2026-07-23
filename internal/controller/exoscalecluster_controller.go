/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// ExoscaleClusterReconciler reconciles a ExoscaleCluster object
type ExoscaleClusterReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	WatchFilter       string
	NewClusterService func(apiKey, apiSecret string, zone egoscale.ZoneName, logger logr.Logger) (domain.ClusterService, error)
}

// Read CAPI cluster resource
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Manage infra exoscale cluster resources
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/finalizers,verbs=update

// Read secrets for Exoscale API credentials
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// - implement condition cf. deprecated: "sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions/v1beta2" might be replaced by "https://pkg.go.dev/sigs.k8s.io/cluster-api/util/conditions"
// - find a way to start a reconciliation loop when cluster is ready in order to create and deploy a secret with an exoscale api key/secret + refresh (for csi and ccm)
func (r *ExoscaleClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := logf.FromContext(ctx)

	log.Info("Reconcile cluster", "req", req)

	// fetch exoscale cluster.
	var exoCluster infrav1alpha1.ExoscaleCluster
	if err := r.Get(ctx, req.NamespacedName, &exoCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	patchHelper, err := patch.NewHelper(&exoCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	skipDeferredPatch := false
	defer func() {
		if skipDeferredPatch {
			return
		}
		// TODO: maybe create a real error management with custom type
		if reterr != nil {
			conditions.Set(&exoCluster, metav1.Condition{
				Type:    infrav1alpha1.ReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1alpha1.ReconcileErrorReason,
				Message: reterr.Error(),
			})
		}
		if err := patchHelper.Patch(ctx, &exoCluster); err != nil {
			log.Error(err, "unable to patch cluster", "cluster name", exoCluster.Name, "cluster id", exoCluster.Status.ID)
			reterr = errors.Join(reterr, fmt.Errorf("unable to patch cluster: %w", err))
		}
	}()

	clusterIDChanged, err := ensureClusterID(&exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if clusterIDChanged {
		if err := patchHelper.Patch(ctx, &exoCluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	log = log.WithValues("cluster_id", *exoCluster.Status.ID)

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, exoCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{}, nil
	}

	// check if we need to continue the reconciliation.
	if annotations.IsExternallyManaged(cluster) {
		log.Info("Cluster is externally managed, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if annotations.IsPaused(cluster, &exoCluster) {
		log.Info("InfraCluster is paused, skipping reconciliation")
		conditions.Set(&exoCluster, metav1.Condition{
			Type:    clusterv1.PausedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.PausedReason,
			Message: "Reconciliation is paused",
		})
		return ctrl.Result{}, nil
	}
	conditions.Delete(&exoCluster, clusterv1.PausedCondition)

	// fetch creds affiliated to this cluster.
	var creds v1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: exoCluster.Spec.ExoscaleSecret.Name}, &creds); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch exoscale secret %q: %w", exoCluster.Spec.ExoscaleSecret.Name, err)
	}
	apiKey, apiSecret, err := GetAPICreds(creds, exoCluster.Spec.ExoscaleSecret.ApiKey, exoCluster.Spec.ExoscaleSecret.APISecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// reconcile cluster.
	clusterSvc, err := r.NewClusterService(apiKey, apiSecret, exoCluster.Spec.Zone, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !exoCluster.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer) {
			exoCluster, err = clusterSvc.DeleteCluster(ctx, exoCluster)
			if err != nil {
				return ctrl.Result{}, err
			}
			log.Info("DELETE FINALIZER")
			beforeFinalizerRemoval := exoCluster.DeepCopy()
			controllerutil.RemoveFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)
			skipDeferredPatch = true
			return ctrl.Result{}, client.IgnoreNotFound(r.Patch(ctx, &exoCluster, client.MergeFrom(beforeFinalizerRemoval)))
		}
		skipDeferredPatch = true
		return ctrl.Result{}, nil
	} else {
		controllerutil.AddFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)
	}

	exoCluster, err = clusterSvc.ReconcileCluster(ctx, exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO: create role + api key/secret when exoscale.status.initialization.provisioned == true and capicluster is ready
	// possible since we watch the capi cluster now (cf. SetupWithManager).

	conditions.Set(&exoCluster, metav1.Condition{
		Type:   infrav1alpha1.ReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: infrav1alpha1.ReconcileSuccessReason,
	})

	return ctrl.Result{}, nil
}

func ensureClusterID(exoCluster *infrav1alpha1.ExoscaleCluster) (bool, error) {
	annotationID := exoCluster.Annotations[domain.ClusterIDKey]
	statusID := ""
	if exoCluster.Status.ID != nil {
		statusID = *exoCluster.Status.ID
	}
	if annotationID != "" && statusID != "" && annotationID != statusID {
		return false, fmt.Errorf("cluster ID annotation %q does not match status %q", annotationID, statusID)
	}

	clusterID := annotationID
	if clusterID == "" {
		clusterID = statusID
	}
	if clusterID == "" {
		clusterID = uuid.NewString()
	}
	if _, err := uuid.Parse(clusterID); err != nil {
		return false, fmt.Errorf("invalid cluster ID %q: %w", clusterID, err)
	}

	changed := annotationID != clusterID || statusID != clusterID
	if exoCluster.Annotations == nil {
		exoCluster.Annotations = map[string]string{}
	}
	exoCluster.Annotations[domain.ClusterIDKey] = clusterID
	exoCluster.Status.ID = &clusterID
	return changed, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExoscaleClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch Cluster objects and map them to the ExoscaleCluster they own.
	// This is required so that lifecycle events on the CAPI Cluster (pause, unpause, owner-ref set)
	// trigger reconciliation of the corresponding ExoscaleCluster. Without this watch the controller
	// only reacts to direct changes on ExoscaleCluster resources and would, for example, never notice
	// when a Cluster is paused or when the owner reference is initially set by the Cluster controller.
	clusterToExoscaleCluster := util.ClusterToInfrastructureMapFunc(
		context.Background(),
		infrav1alpha1.GroupVersion.WithKind("ExoscaleCluster"),
		mgr.GetClient(),
		&infrav1alpha1.ExoscaleCluster{},
	)

	blder := ctrl.NewControllerManagedBy(mgr).Named("exoscalecluster")

	if r.WatchFilter != "" {
		blder = blder.For(&infrav1alpha1.ExoscaleCluster{}, builder.WithPredicates(
			predicates.ResourceHasFilterLabel(r.Scheme, logf.Log, r.WatchFilter),
		))
	} else {
		blder = blder.For(&infrav1alpha1.ExoscaleCluster{})
	}

	return blder.
		Watches(&clusterv1.Cluster{}, handler.EnqueueRequestsFromMapFunc(clusterToExoscaleCluster)).
		Complete(r)
}
