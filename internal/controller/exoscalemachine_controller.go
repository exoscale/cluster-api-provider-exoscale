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
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	capilabels "sigs.k8s.io/cluster-api/util/labels"
	capipatch "sigs.k8s.io/cluster-api/util/patch"
	capipredicates "sigs.k8s.io/cluster-api/util/predicates"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
	"github.com/go-logr/logr"
)

// machineFinalizer guards deletion of an ExoscaleMachine until the underlying
// Exoscale instance has been removed. It must match the value that other
// components (CLI, webhooks) might reference, so it lives next to the
// reconciler that owns the lifecycle.
const machineFinalizer = "exoscalemachine.infrastructure.cluster.x-k8s.io"

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/finalizers,verbs=update

// ExoscaleMachineReconciler reconciles an ExoscaleMachine object.
type ExoscaleMachineReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	WatchFilter        string
	NewInstanceService func(apiKey, apiSecret string, zone egoscale.ZoneName, logger logr.Logger) (domain.InstanceService, error)
}

func (r *ExoscaleMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := logf.FromContext(ctx)
	exoMachine := &infrastructurev1alpha1.ExoscaleMachine{}
	if err := r.Get(ctx, req.NamespacedName, exoMachine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.WatchFilter != "" && !capilabels.HasWatchLabel(exoMachine, r.WatchFilter) {
		return ctrl.Result{}, nil
	}

	patchHelper, err := capipatch.NewHelper(exoMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if pErr := patchHelper.Patch(ctx, exoMachine); pErr != nil && reterr == nil {
			reterr = pErr
		}
	}()

	deleting := !exoMachine.DeletionTimestamp.IsZero()
	var machine *clusterv1.Machine
	var machineID, instanceID *uuid.UUID
	cluster, clusterErr := util.GetClusterFromMetadata(ctx, r.Client, exoMachine.ObjectMeta)
	if deleting {
		machineID, instanceID, err = deletionInstanceIDs(exoMachine)
		if err != nil {
			return ctrl.Result{}, err
		}
		if machineID == nil && instanceID == nil {
			controllerutil.RemoveFinalizer(exoMachine, machineFinalizer)
			return ctrl.Result{}, nil
		}
		if clusterErr != nil {
			return ctrl.Result{}, clusterErr
		}
	} else if clusterErr != nil {
		machine, err = util.GetOwnerMachine(ctx, r.Client, exoMachine.ObjectMeta)
		if err != nil {
			return ctrl.Result{}, err
		}
		if machine == nil {
			log.Info("owner Machine not yet set, requeueing")
			return ctrl.Result{}, nil
		}
		cluster, err = util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
		if err != nil {
			log.Info("Machine missing cluster label or cluster not found")
			return ctrl.Result{}, nil
		}
	}

	if annotations.IsPaused(cluster, exoMachine) {
		log.Info("ExoscaleMachine or Cluster is paused")
		conditions.Set(exoMachine, metav1.Condition{
			Type:    clusterv1.PausedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.PausedReason,
			Message: "Reconciliation is paused",
		})
		return ctrl.Result{}, nil
	}
	conditions.Delete(exoMachine, clusterv1.PausedCondition)

	if deleting {
		exoCluster, err := r.getExoscaleCluster(ctx, cluster)
		if err != nil {
			return ctrl.Result{}, err
		}

		instanceService, err := r.instanceService(ctx, exoCluster)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.reconcileDelete(ctx, exoMachine, instanceService, machineID, instanceID)
	}
	if machine == nil {
		machine, err = util.GetOwnerMachine(ctx, r.Client, exoMachine.ObjectMeta)
		if err != nil {
			return ctrl.Result{}, err
		}
		if machine == nil {
			log.Info("owner Machine not yet set, requeueing")
			return ctrl.Result{}, nil
		}
	}

	exoCluster, err := r.getExoscaleCluster(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if controllerutil.AddFinalizer(exoMachine, machineFinalizer) {
		if err := patchHelper.Patch(ctx, exoMachine); err != nil {
			return ctrl.Result{}, err
		}
	}
	if cluster.Status.Initialization.InfrastructureProvisioned == nil || !*cluster.Status.Initialization.InfrastructureProvisioned {
		log.Info("cluster infrastructure not yet ready")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("bootstrap data not yet available")
		return ctrl.Result{}, nil
	}

	instanceService, err := r.instanceService(ctx, exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	return r.reconcileNormal(ctx, exoMachine, machine, exoCluster, instanceService, patchHelper)
}

func (r *ExoscaleMachineReconciler) getExoscaleCluster(ctx context.Context, cluster *clusterv1.Cluster) (*infrastructurev1alpha1.ExoscaleCluster, error) {
	if !cluster.Spec.InfrastructureRef.IsDefined() {
		return nil, fmt.Errorf("cluster %q has no infrastructureRef", cluster.Name)
	}

	exoCluster := &infrastructurev1alpha1.ExoscaleCluster{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Spec.InfrastructureRef.Name}, exoCluster); err != nil {
		return nil, fmt.Errorf("unable to fetch ExoscaleCluster %q: %w", cluster.Spec.InfrastructureRef.Name, err)
	}
	return exoCluster, nil
}

func (r *ExoscaleMachineReconciler) instanceService(
	ctx context.Context,
	exoCluster *infrastructurev1alpha1.ExoscaleCluster,
) (domain.InstanceService, error) {
	if r.NewInstanceService == nil {
		return nil, fmt.Errorf("NewInstanceService is not wired")
	}

	creds := &corev1.Secret{}
	secretRef := exoCluster.Spec.ExoscaleSecret
	if err := r.Get(ctx, types.NamespacedName{Namespace: exoCluster.Namespace, Name: secretRef.Name}, creds); err != nil {
		return nil, fmt.Errorf("unable to fetch exoscale secret %q: %w", secretRef.Name, err)
	}

	apiKey, apiSecret, err := GetAPICreds(*creds, secretRef.ApiKey, secretRef.APISecret)
	if err != nil {
		return nil, err
	}

	return r.NewInstanceService(apiKey, apiSecret, exoCluster.Spec.Zone, logf.FromContext(ctx))
}

func (r *ExoscaleMachineReconciler) reconcileNormal(
	ctx context.Context,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
	machine *clusterv1.Machine,
	exoCluster *infrastructurev1alpha1.ExoscaleCluster,
	instanceService domain.InstanceService,
	patchHelper *capipatch.Helper,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.AddFinalizer(exoMachine, machineFinalizer) {
		if err := patchHelper.Patch(ctx, exoMachine); err != nil {
			return ctrl.Result{}, err
		}
	}

	userData, err := r.bootstrapData(ctx, machine)
	if err != nil {
		return ctrl.Result{}, err
	}

	machineUID, err := uuid.Parse(string(machine.UID))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid Machine UID %q: %w", machine.UID, err)
	}

	var instanceID *uuid.UUID
	if exoMachine.Status.InstanceID != "" {
		id, err := uuid.Parse(exoMachine.Status.InstanceID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid instanceID in status %q: %w", exoMachine.Status.InstanceID, err)
		}
		instanceID = &id
	}

	securityGroupIDs, err := securityGroupIDs(exoCluster, exoMachine)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(securityGroupIDs) == 0 {
		log.Info("cluster node security group not yet available")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	spec := domain.InstanceSpec{
		Name:             machine.Name,
		Template:         exoMachine.Spec.Template,
		InstanceType:     exoMachine.Spec.InstanceType,
		SSHKey:           exoMachine.Spec.SSHKey,
		SecurityGroupIDs: securityGroupIDs,
		RootVolumeSizeGB: exoMachine.Spec.RootVolumeSizeGB,
		UserData:         userData,
	}

	instance, err := instanceService.UpsertInstance(ctx, machineUID, instanceID, spec)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("upsert instance: %w", err)
	}

	exoMachine.Status.InstanceID = instance.ID.String()
	exoMachine.Status.InstanceState = instance.State

	if instance.State != "running" {
		log.Info("instance not yet running", "state", instance.State)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	providerID := fmt.Sprintf("exoscale://%s", instance.ID)
	exoMachine.Spec.ProviderID = &providerID
	exoMachine.Status.Ready = true
	exoMachine.Status.Initialization.Provisioned = new(true)
	exoMachine.Status.Addresses = buildAddresses(instance)

	return ctrl.Result{}, nil
}

func (r *ExoscaleMachineReconciler) reconcileDelete(
	ctx context.Context,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
	instanceService domain.InstanceService,
	machineID, instanceID *uuid.UUID,
) error {
	if err := instanceService.DeleteInstance(ctx, machineID, instanceID); err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
		return fmt.Errorf("delete instance: %w", err)
	}

	controllerutil.RemoveFinalizer(exoMachine, machineFinalizer)
	return nil
}

func deletionInstanceIDs(exoMachine *infrastructurev1alpha1.ExoscaleMachine) (*uuid.UUID, *uuid.UUID, error) {
	var instanceID *uuid.UUID
	if exoMachine.Status.InstanceID != "" {
		id, err := uuid.Parse(exoMachine.Status.InstanceID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid instanceID in status %q: %w", exoMachine.Status.InstanceID, err)
		}
		instanceID = &id
	}

	for _, ref := range exoMachine.OwnerReferences {
		if ref.Kind != "Machine" {
			continue
		}
		groupVersion, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid owner API version %q: %w", ref.APIVersion, err)
		}
		if groupVersion.Group != clusterv1.GroupVersion.Group {
			continue
		}
		machineID, err := uuid.Parse(string(ref.UID))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid Machine owner UID %q: %w", ref.UID, err)
		}
		return &machineID, instanceID, nil
	}

	return nil, instanceID, nil
}

func (r *ExoscaleMachineReconciler) bootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, error) {
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", fmt.Errorf("bootstrap data secret name is not set")
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: machine.Namespace, Name: *machine.Spec.Bootstrap.DataSecretName}, secret); err != nil {
		return "", fmt.Errorf("unable to fetch bootstrap data secret %q: %w", *machine.Spec.Bootstrap.DataSecretName, err)
	}

	data, ok := secret.Data["value"]
	if !ok {
		return "", fmt.Errorf("bootstrap data secret %q has no value key", *machine.Spec.Bootstrap.DataSecretName)
	}
	return string(data), nil
}

func securityGroupIDs(
	exoCluster *infrastructurev1alpha1.ExoscaleCluster,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(exoMachine.Spec.SecurityGroups)+1)
	if exoCluster.Status.SecurityGroupNode != nil {
		id, err := uuid.Parse(exoCluster.Status.SecurityGroupNode.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid node security group ID %q: %w", exoCluster.Status.SecurityGroupNode.ID, err)
		}
		ids = append(ids, id)
	}

	for _, rawID := range exoMachine.Spec.SecurityGroups {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid ExoscaleMachine security group ID %q: %w", rawID, err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExoscaleMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicateLog := mgr.GetLogger().WithValues("controller", "exoscalemachine")
	clusterToExoscaleMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrastructurev1alpha1.ExoscaleMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	return capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&infrastructurev1alpha1.ExoscaleMachine{}).
		WithEventFilter(capipredicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilter)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(
					infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine"),
				),
			),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToExoscaleMachines),
			capipredicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), predicateLog),
			capipredicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilter),
		).
		Named("exoscalemachine").
		Complete(r)
}

func buildAddresses(instance domain.Instance) []clusterv1.MachineAddress {
	var addrs []clusterv1.MachineAddress
	if instance.PublicIP != "" {
		addrs = append(addrs, clusterv1.MachineAddress{
			Type:    clusterv1.MachineExternalIP,
			Address: instance.PublicIP,
		})
	}
	if instance.PrivateIP != "" {
		addrs = append(addrs, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: instance.PrivateIP,
		})
	}
	return addrs
}
