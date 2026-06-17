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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/service"
	"github.com/google/uuid"
)

// ExoscaleClusterReconciler reconciles a ExoscaleCluster object
type ExoscaleClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Read CAPI cluster resource
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Manage infra exoscale cluster resources
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters/finalizers,verbs=update

// Read secrets for Exoscale API credentials
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ExoscaleCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile

// TODO:
// - get exoscale cluster resource
// - skip reconciliation if paused
// - create eip or nlb
// - create security group
// - return error log + nil error + requeu or requeue after ctrl.Result
// - implement condition cf. deprecated: "sigs.k8s.io/cluster-api/util/deprecated/v1beta1/conditions/v1beta2" might be replaced by "https://pkg.go.dev/sigs.k8s.io/cluster-api/util/conditions"
// - find a way to start a reconciliation loop when cluster is ready in order to create and deploy a secret with an exoscale api key/secret + refresh (for csi and ccm)
func (r *ExoscaleClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
	defer func() {
		if err := patchHelper.Patch(ctx, &exoCluster); err != nil {
			log.Error(err, "unable to patch cluster", "cluster name", exoCluster.Name, "cluster id", exoCluster.Status.ID)
		}
	}()

	if exoCluster.Status.ID == nil {
		exoCluster.Status.ID = func() *string { v := uuid.New().String(); return &v }()
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
		return ctrl.Result{}, nil
	}

	// fetch creds affiliated to this cluster.
	var creds v1.Secret
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: exoCluster.Spec.ExoscaleSecret.Name}, &creds); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch exoscale secret %q: %w", exoCluster.Spec.ExoscaleSecret.Name, err)
	}
	apiKey, apiSecret, err := GetAPICreds(creds, exoCluster.Spec.ExoscaleSecret.ApiKey, exoCluster.Spec.ExoscaleSecret.APISecret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// reconcile cluster.
	clusterSvc, err := service.NewClusterService(apiKey, apiSecret, exoCluster.Spec.Zone, log)
	if err != nil {
		log.Error(errors.New("invalid creds error"), "invalid creds", "apikey", apiKey, "apiSecret", apiSecret)
		return ctrl.Result{}, err
	}

	if !exoCluster.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer) {
			exoCluster, err = clusterSvc.DeleteCluster(ctx, exoCluster)
			if err != nil {
				return ctrl.Result{}, err
			}
			log.Info("DELETE FINALIZER")
			controllerutil.RemoveFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)
		}
		return ctrl.Result{}, nil
	} else {
		controllerutil.AddFinalizer(&exoCluster, infrav1alpha1.ExoscaleClusterFinalizer)
	}

	exoCluster, err = clusterSvc.ReconcileCluster(ctx, exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	// use patchHelper. helper.Patch(ctx, &exoCluster) after the update of the exoCluster object to persist the changes

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExoscaleClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.ExoscaleCluster{}).
		Named("exoscalecluster").
		Complete(r)
}
