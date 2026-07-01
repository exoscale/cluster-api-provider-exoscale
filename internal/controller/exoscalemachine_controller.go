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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	capipatch "sigs.k8s.io/cluster-api/util/patch"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
)

const machineFinalizer = "exoscalemachine.infrastructure.cluster.x-k8s.io"

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/finalizers,verbs=update

// ExoscaleMachineReconciler reconciles an ExoscaleMachine object.
type ExoscaleMachineReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	InstanceService domain.InstanceService
}

func (r *ExoscaleMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := logf.FromContext(ctx)

	exoMachine := &infrastructurev1alpha1.ExoscaleMachine{}
	if err := r.Get(ctx, req.NamespacedName, exoMachine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, exoMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("owner Machine not yet set, requeueing")
		return ctrl.Result{}, nil
	}

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("Machine missing cluster label or cluster not found")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(cluster, exoMachine) {
		log.Info("ExoscaleMachine or Cluster is paused")
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

	if !exoMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, exoMachine)
	}
	return r.reconcileNormal(ctx, exoMachine, machine, patchHelper)
}

func (r *ExoscaleMachineReconciler) reconcileNormal(
	ctx context.Context,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
	machine *clusterv1.Machine,
	patchHelper *capipatch.Helper,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.InstanceService == nil {
		return ctrl.Result{}, fmt.Errorf("InstanceService is not wired: provide an ExoscaleClient adapter")
	}

	if controllerutil.AddFinalizer(exoMachine, machineFinalizer) {
		if err := patchHelper.Patch(ctx, exoMachine); err != nil {
			return ctrl.Result{}, err
		}
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("bootstrap data not yet available")
		return ctrl.Result{}, nil
	}

	templateID, err := uuid.Parse(exoMachine.Spec.TemplateID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid templateID %q: %w", exoMachine.Spec.TemplateID, err)
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

	spec := domain.InstanceSpec{
		Name:             machine.Name,
		Zone:             exoMachine.Spec.Zone,
		TemplateID:       templateID,
		InstanceType:     exoMachine.Spec.InstanceType,
		SSHKey:           exoMachine.Spec.SSHKey,
		RootVolumeSizeGB: exoMachine.Spec.RootVolumeSizeGB,
	}

	instance, err := r.InstanceService.UpsertInstance(ctx, machineUID, instanceID, spec)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("upsert instance: %w", err)
	}

	exoMachine.Status.InstanceID = instance.ID.String()
	exoMachine.Status.InstanceState = instance.State

	if instance.State != "running" {
		log.Info("instance not yet running", "state", instance.State)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	providerID := fmt.Sprintf("exoscale:///%s", instance.ID)
	exoMachine.Spec.ProviderID = &providerID
	exoMachine.Status.Ready = true
	exoMachine.Status.Addresses = buildAddresses(instance)

	return ctrl.Result{}, nil
}

func (r *ExoscaleMachineReconciler) reconcileDelete(
	ctx context.Context,
	exoMachine *infrastructurev1alpha1.ExoscaleMachine,
) (ctrl.Result, error) {
	if exoMachine.Status.InstanceID == "" {
		controllerutil.RemoveFinalizer(exoMachine, machineFinalizer)
		return ctrl.Result{}, nil
	}

	instanceID, err := uuid.Parse(exoMachine.Status.InstanceID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid instanceID in status %q: %w", exoMachine.Status.InstanceID, err)
	}

	if r.InstanceService != nil {
		if err := r.InstanceService.DeleteInstance(ctx, instanceID); err != nil && !errors.Is(err, domain.ErrInstanceNotFound) {
			return ctrl.Result{}, fmt.Errorf("delete instance: %w", err)
		}
	}

	controllerutil.RemoveFinalizer(exoMachine, machineFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExoscaleMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ExoscaleMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(
					infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine"),
				),
			),
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
