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
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
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
	"github.com/exoscale/cluster-api-provider-exoscale/internal/service"
	"github.com/google/uuid"
)

type ExoscaleClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	WatchFilter string
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// ReadyCondition is the CAPI-standard condition type reported on the ExoscaleCluster status.
// See: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster#infracluster-status-conditions
const ReadyCondition = "Ready"

func (r *ExoscaleClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := logf.FromContext(ctx)

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
	defer func() {
		if reterr != nil {
			apimeta.SetStatusCondition(&exoCluster.Status.Conditions, metav1.Condition{
				Type:               ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             "ReconcileError",
				Message:            reterr.Error(),
				ObservedGeneration: exoCluster.Generation,
			})
		}
		if err := patchHelper.Patch(ctx, &exoCluster); err != nil {
			log.Error(err, "Unable to patch ExoscaleCluster",
				"name", exoCluster.Name,
				"id", exoCluster.Status.ID)
		}
	}()

	if exoCluster.Status.ID == nil {
		id := uuid.New().String()
		exoCluster.Status.ID = &id
	}

	log = log.WithValues("cluster_id", *exoCluster.Status.ID)

	cluster, err := util.GetOwnerCluster(ctx, r.Client, exoCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{}, nil
	}

	if annotations.IsExternallyManaged(cluster) {
		log.Info("Cluster is externally managed, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if annotations.IsPaused(cluster, &exoCluster) {
		log.Info("ExoscaleCluster is paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	var creds v1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      exoCluster.Spec.ExoscaleSecret.Name,
	}, &creds); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch exoscale secret %q: %w", exoCluster.Spec.ExoscaleSecret.Name, err)
	}
	apiKey, apiSecret, err := GetAPICreds(creds, exoCluster.Spec.ExoscaleSecret.APIKey, exoCluster.Spec.ExoscaleSecret.APISecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterSvc, err := service.NewClusterService(apiKey, apiSecret, exoCluster.Spec.Zone, log)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to create cluster service: %w", err)
	}

	if !exoCluster.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer) {
			exoCluster, err = clusterSvc.DeleteCluster(ctx, exoCluster)
			if err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)
		}
		return ctrl.Result{}, nil
	}
	controllerutil.AddFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)

	exoCluster, err = clusterSvc.ReconcileCluster(ctx, exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO(sc-184544): provision a scoped Exoscale API key/secret into the workload cluster once
	// status.initialization.provisioned is true and the CAPI Cluster is ready. We already watch
	// the owner Cluster (see SetupWithManager), so the reconcile loop will fire again.

	apimeta.SetStatusCondition(&exoCluster.Status.Conditions, metav1.Condition{
		Type:               ReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileSuccess",
		ObservedGeneration: exoCluster.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *ExoscaleClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch Cluster objects and map them to the ExoscaleCluster they own.
	// Without this, lifecycle events on the CAPI Cluster (pause, unpause, owner-ref set)
	// would never trigger reconciliation of the corresponding ExoscaleCluster.
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
