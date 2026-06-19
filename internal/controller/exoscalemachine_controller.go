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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

// Read CAPI machine resource
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch

// Manage infra exoscale machine resources
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines/finalizers,verbs=update

// ExoscaleMachineReconciler reconciles an ExoscaleMachine object.
type ExoscaleMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ExoscaleMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO: reconcile the Exoscale instance backing this Machine.

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExoscaleMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ExoscaleMachine{}).
		Named("exoscalemachine").
		Complete(r)
}
