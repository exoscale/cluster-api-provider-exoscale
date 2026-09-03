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
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
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
func (*ExoscaleMachineCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	_, claimed := obj.Annotations[domain.MachineUIDKey]
	if claimed == (obj.Spec.ProviderID != nil) {
		return nil, nil
	}
	var err *field.Error
	if obj.Spec.ProviderID != nil {
		err = field.Required(field.NewPath("metadata", "annotations").Key(domain.MachineUIDKey), "required for a moved machine")
	} else {
		err = field.Required(field.NewPath("spec", "providerID"), "required when the machine UID annotation is set")
	}
	return nil, apierrors.NewInvalid(
		infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine").GroupKind(),
		obj.Name,
		field.ErrorList{err},
	)
}

// ValidateUpdate implements admission.Validator.
func (*ExoscaleMachineCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	oldSpec := oldObj.Spec
	newSpec := newObj.Spec

	// The controller sets providerID after creating the instance.
	oldSpec.ProviderID = nil
	newSpec.ProviderID = nil

	var allErrs field.ErrorList
	if !reflect.DeepEqual(oldSpec, newSpec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "instance creation fields are immutable"))
	}
	_, deletingForMove := newObj.Annotations[clusterctlv1.DeleteForMoveAnnotation]
	if !deletingForMove && oldObj.Annotations[domain.MachineUIDKey] != "" && oldObj.Annotations[domain.MachineUIDKey] != newObj.Annotations[domain.MachineUIDKey] {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("metadata", "annotations").Key(domain.MachineUIDKey), "machine UID is immutable"))
	}
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachine").GroupKind(),
			newObj.Name,
			allErrs,
		)
	}

	return nil, nil
}

// ValidateDelete implements admission.Validator.
func (*ExoscaleMachineCustomValidator) ValidateDelete(_ context.Context, _ *infrastructurev1alpha1.ExoscaleMachine) (admission.Warnings, error) {
	return nil, nil
}
