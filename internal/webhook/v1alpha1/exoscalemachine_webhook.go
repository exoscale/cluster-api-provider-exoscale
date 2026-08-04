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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

// SetupExoscaleMachineWebhookWithManager registers the webhook for ExoscaleMachine in the manager.
func SetupExoscaleMachineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleMachine{}).
		WithValidator(&ExoscaleMachineCustomValidator{}).
		Complete()
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

	// The controller sets providerID after creating the instance.
	oldSpec.ProviderID = nil
	newSpec.ProviderID = nil

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
