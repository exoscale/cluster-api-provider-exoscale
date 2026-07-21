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
	"crypto/sha256"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
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

	patchHelper, err := capipatch.NewHelper(exoMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() { reterr = patchExoscaleMachine(ctx, patchHelper, exoMachine, reterr) }()

	if annotations.HasPaused(exoMachine) {
		setMachinePaused(exoMachine)
		return ctrl.Result{}, nil
	}

	deleting := !exoMachine.DeletionTimestamp.IsZero()
	var machine *clusterv1.Machine
	cluster, clusterErr := util.GetClusterFromMetadata(ctx, r.Client, exoMachine.ObjectMeta)
	if !deleting && clusterErr != nil {
		machine, err = util.GetOwnerMachine(ctx, r.Client, exoMachine.ObjectMeta)
		if err != nil {
			return ctrl.Result{}, err
		}
		if machine == nil {
			log.Info("owner Machine not yet set, requeueing")
			setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.NotReadyReason, "Waiting for owner Machine")
			return ctrl.Result{}, nil
		}
		cluster, clusterErr = util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
		if clusterErr != nil {
			log.Info("Machine missing cluster label or cluster not found")
			setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.NotReadyReason, "Waiting for Cluster")
			return ctrl.Result{}, nil
		}
	}

	if clusterErr == nil && annotations.IsPaused(cluster, exoMachine) {
		log.Info("ExoscaleMachine or Cluster is paused")
		setMachinePaused(exoMachine)
		return ctrl.Result{}, nil
	}
	conditions.Delete(exoMachine, clusterv1.PausedCondition)

	if deleting {
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.DeletingReason, "Deleting instance")
		machineID, instanceID, err := deletionInstanceIDs(exoMachine)
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
			setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.NotReadyReason, "Waiting for owner Machine")
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
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.WaitingForClusterInfrastructureReadyReason, "Waiting for cluster infrastructure")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("bootstrap data not yet available")
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.WaitingForBootstrapDataReason, "Waiting for bootstrap data")
		return ctrl.Result{}, nil
	}

	instanceService, err := r.instanceService(ctx, exoCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	return r.reconcileNormal(ctx, exoMachine, machine, exoCluster, instanceService, patchHelper)
}

func setMachinePaused(exoMachine *infrastructurev1alpha1.ExoscaleMachine) {
	conditions.Set(exoMachine, metav1.Condition{
		Type:    clusterv1.PausedCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.PausedReason,
		Message: "Reconciliation is paused",
	})
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

	machineID := domain.MachineID(machine.UID)
	if machineID == "" {
		return ctrl.Result{}, fmt.Errorf("machine UID is empty")
	}

	var instanceID *uuid.UUID
	if exoMachine.Status.InstanceID != "" {
		id, err := uuid.Parse(exoMachine.Status.InstanceID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid instanceID in status %q: %w", exoMachine.Status.InstanceID, err)
		}
		instanceID = &id
	}

	isControlPlane := util.IsControlPlaneMachine(machine)
	securityGroupIDs, err := securityGroupIDs(exoCluster, exoMachine, isControlPlane)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(securityGroupIDs) == 0 {
		log.Info("Cluster machine security group not yet available", "controlPlane", isControlPlane)
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.NotReadyReason, "Waiting for machine security group")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	var elasticIPID *uuid.UUID
	if isControlPlane {
		if exoCluster.Status.ControlPlaneEndpoint == nil {
			return ctrl.Result{}, fmt.Errorf("cluster control plane Elastic IP is not available")
		}
		id, err := uuid.Parse(exoCluster.Status.ControlPlaneEndpoint.ID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid control plane Elastic IP ID %q: %w", exoCluster.Status.ControlPlaneEndpoint.ID, err)
		}
		elasticIPID = &id
	}

	spec := domain.InstanceSpec{
		Name:              instanceName(machine.Namespace, machine.Name),
		Template:          exoMachine.Spec.Template,
		InstanceType:      exoMachine.Spec.InstanceType,
		SSHKey:            exoMachine.Spec.SSHKey,
		SecurityGroupIDs:  securityGroupIDs,
		ElasticIPID:       elasticIPID,
		RootVolumeSizeGiB: exoMachine.Spec.RootVolumeSizeGiB,
		UserData:          userData,
	}

	instance, err := instanceService.UpsertInstance(ctx, machineID, instanceID, spec)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("upsert instance: %w", err)
	}

	exoMachine.Status.InstanceID = instance.ID.String()
	if instance.State != "running" {
		log.Info("instance not yet running", "state", instance.State)
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.NotReadyReason, fmt.Sprintf("Instance state is %s", instance.State))
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	providerID := fmt.Sprintf("exoscale://%s", instance.ID)
	exoMachine.Spec.ProviderID = &providerID
	exoMachine.Status.Initialization.Provisioned = new(true)
	exoMachine.Status.Addresses = buildAddresses(instance)
	setMachineReady(exoMachine, metav1.ConditionTrue, clusterv1.ReadyReason, "Instance is running")

	return ctrl.Result{}, nil
}

func instanceName(namespace, name string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(namespace+"\x00"+name)))[:8]
	base := namespace + "-" + name
	if len(base) > 255-len(hash)-1 {
		base = base[:255-len(hash)-1]
	}
	return base + "-" + hash
}

func (r *ExoscaleMachineReconciler) reconcileDelete(
	ctx context.Context,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
	instanceService domain.InstanceService,
	machineID *domain.MachineID,
	instanceID *uuid.UUID,
) error {
	if err := instanceService.DeleteInstance(ctx, machineID, instanceID); err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
		return fmt.Errorf("delete instance: %w", err)
	}

	controllerutil.RemoveFinalizer(exoMachine, machineFinalizer)
	return nil
}

func deletionInstanceIDs(exoMachine *infrastructurev1alpha1.ExoscaleMachine) (*domain.MachineID, *uuid.UUID, error) {
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
		if ref.UID == "" {
			return nil, instanceID, nil
		}
		machineID := domain.MachineID(ref.UID)
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
	if len(data) == 0 {
		return "", fmt.Errorf("bootstrap data secret %q has an empty value", *machine.Spec.Bootstrap.DataSecretName)
	}
	return string(data), nil
}

func securityGroupIDs(
	exoCluster *infrastructurev1alpha1.ExoscaleCluster,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
	isControlPlane bool,
) ([]uuid.UUID, error) {
	managedSecurityGroup := exoCluster.Status.SecurityGroupNode
	role := "node"
	if isControlPlane {
		managedSecurityGroup = exoCluster.Status.SecurityGroupControlPlan
		role = "control plane"
	}
	if managedSecurityGroup == nil {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(exoMachine.Spec.SecurityGroups)+1)
	id, err := uuid.Parse(managedSecurityGroup.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid %s security group ID %q: %w", role, managedSecurityGroup.ID, err)
	}
	ids = append(ids, id)

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
	machineToExoscaleMachine := filterExoscaleMachineRequests(
		mgr.GetClient(),
		r.WatchFilter,
		util.MachineToInfrastructureMapFunc(infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine")),
	)
	clusterToExoscaleMachines = filterExoscaleMachineRequests(mgr.GetClient(), r.WatchFilter, clusterToExoscaleMachines)

	return capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&infrastructurev1alpha1.ExoscaleMachine{}).
		WithEventFilter(capipredicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilter)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(machineToExoscaleMachine),
		).
		// Cluster readiness and pause changes can affect every infrastructure machine in the cluster.
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToExoscaleMachines),
			capipredicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), predicateLog),
			capipredicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilter),
		).
		// ExoscaleCluster status provides the shared security group and endpoint IDs required by its machines.
		Watches(
			&infrastructurev1alpha1.ExoscaleCluster{},
			handler.EnqueueRequestsFromMapFunc(exoscaleClusterToExoscaleMachines(mgr.GetClient(), clusterToExoscaleMachines)),
		).
		Named("exoscalemachine").
		Complete(r)
}

func filterExoscaleMachineRequests(c client.Client, watchFilter string, mapper handler.MapFunc) handler.MapFunc {
	if watchFilter == "" {
		return mapper
	}
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		requests := mapper(ctx, obj)
		filtered := make([]reconcile.Request, 0, len(requests))
		for _, request := range requests {
			exoMachine := &infrastructurev1alpha1.ExoscaleMachine{}
			if err := c.Get(ctx, request.NamespacedName, exoMachine); err != nil {
				continue
			}
			if exoMachine.Labels[clusterv1.WatchLabel] == watchFilter {
				filtered = append(filtered, request)
			}
		}
		return filtered
	}
}

func exoscaleClusterToExoscaleMachines(c client.Client, clusterToExoscaleMachines handler.MapFunc) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		exoCluster, ok := obj.(*infrastructurev1alpha1.ExoscaleCluster)
		if !ok {
			return nil
		}
		cluster, err := util.GetOwnerCluster(ctx, c, exoCluster.ObjectMeta)
		if err != nil || cluster == nil {
			return nil
		}
		return clusterToExoscaleMachines(ctx, cluster)
	}
}

func patchExoscaleMachine(ctx context.Context, patchHelper *capipatch.Helper, exoMachine *infrastructurev1alpha1.ExoscaleMachine, reconcileErr error) error {
	if reconcileErr != nil {
		setMachineReady(exoMachine, metav1.ConditionFalse, clusterv1.InternalErrorReason, reconcileErr.Error())
	}
	if err := patchHelper.Patch(ctx, exoMachine); err != nil && reconcileErr == nil {
		return err
	}
	return reconcileErr
}

func setMachineReady(exoMachine *infrastructurev1alpha1.ExoscaleMachine, status metav1.ConditionStatus, reason, message string) {
	exoMachine.Status.Ready = status == metav1.ConditionTrue
	conditions.Set(exoMachine, metav1.Condition{
		Type:    clusterv1.ReadyCondition,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
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
