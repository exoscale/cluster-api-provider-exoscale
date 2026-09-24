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

package v1alpha1

import (
	"context"
	"reflect"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

// nolint:unused
var exoscalemachinelog = logf.Log.WithName("exoscalemachine-resource")

// SetupExoscaleMachineWebhookWithManager registers the webhook for ExoscaleMachine in the manager.
func SetupExoscaleMachineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleMachine{}).
		WithValidator(&ExoscaleMachineCustomValidator{}).
		WithDefaulter(&ExoscaleMachineCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscalemachine,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines,verbs=create,versions=v1alpha1,name=mexoscalemachine-v1alpha1.kb.io,admissionReviewVersions=v1

// ExoscaleMachineCustomDefaulter assigns the identity that names and labels the Exoscale
// instance backing an ExoscaleMachine.
type ExoscaleMachineCustomDefaulter struct{}

var _ admission.Defaulter[*infrastructurev1alpha1.ExoscaleMachine] = &ExoscaleMachineCustomDefaulter{}

// Default implements admission.Defaulter.
func (*ExoscaleMachineCustomDefaulter) Default(_ context.Context, obj *infrastructurev1alpha1.ExoscaleMachine) error {
	if obj.Spec.MachineID == "" {
		obj.Spec.MachineID = uuid.New().String()
		exoscaleclusterlog.Info("Assigned machine ID", "name", obj.GetName(), "machineID", obj.Spec.MachineID)
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscalemachine,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachines,verbs=create;update,versions=v1alpha1,name=vexoscalemachine-v1alpha1.kb.io,admissionReviewVersions=v1

// ExoscaleMachineCustomValidator validates ExoscaleMachine resources.
type ExoscaleMachineCustomValidator struct{}

var _ admission.Validator[*infrastructurev1alpha1.ExoscaleMachine] = &ExoscaleMachineCustomValidator{}

// ValidateCreate implements admission.Validator.
func (*ExoscaleMachineCustomValidator) ValidateCreate(_ context.Context, _ *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements admission.Validator.
func (*ExoscaleMachineCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	oldSpec := oldObj.Spec
	newSpec := newObj.Spec

	// Covers both a change and a removal: the identity names the Exoscale instance already
	// provisioned for this machine, so losing it strands that instance.
	if oldSpec.MachineID != "" && newSpec.MachineID != oldSpec.MachineID {
		return nil, apierrors.NewInvalid(
			infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine").GroupKind(),
			newObj.Name,
			field.ErrorList{
				field.Forbidden(
					field.NewPath("spec", "machineID"),
					"machineID cannot be changed or removed once set; it identifies the Exoscale instance provisioned for this machine",
				),
			},
		)
	}

	// The controller sets providerID after creating the instance, and assigns machineID on its
	// first reconciliation when the defaulting webhook is not running (ENABLE_WEBHOOKS=false).
	oldSpec.ProviderID = nil
	newSpec.ProviderID = nil
	oldSpec.MachineID = ""
	newSpec.MachineID = ""

	if !reflect.DeepEqual(oldSpec, newSpec) {
		return nil, apierrors.NewInvalid(
			infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine").GroupKind(),
			newObj.Name,
			field.ErrorList{
				field.Forbidden(field.NewPath("spec"), "instance creation fields are immutable"),
			},
		)
	}

	return nil, nil
}

// ValidateDelete implements admission.Validator.
func (*ExoscaleMachineCustomValidator) ValidateDelete(_ context.Context, _ *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	return nil, nil
}
