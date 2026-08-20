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
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

// SetupExoscaleMachineTemplateWebhookWithManager registers the webhook for ExoscaleMachineTemplate in the manager.
func SetupExoscaleMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleMachineTemplate{}).
		WithValidator(&ExoscaleMachineTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscalemachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachinetemplates,verbs=create;update,versions=v1alpha1,name=vexoscalemachinetemplate-v1alpha1.kb.io,admissionReviewVersions=v1

// ExoscaleMachineTemplateCustomValidator validates ExoscaleMachineTemplate resources.
type ExoscaleMachineTemplateCustomValidator struct{}

var _ admission.Validator[*infrastructurev1alpha1.ExoscaleMachineTemplate] = &ExoscaleMachineTemplateCustomValidator{}

// ValidateCreate implements admission.Validator.
func (*ExoscaleMachineTemplateCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	allErrs := obj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))
	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachineTemplate").GroupKind(),
		obj.Name,
		allErrs,
	)
}

// ValidateUpdate implements admission.Validator.
func (*ExoscaleMachineTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an admission.Request in context: %v", err))
	}

	allErrs := newObj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))
	// CAPI dry-runs updates to detect spec changes before rotating immutable templates:
	// https://github.com/kubernetes-sigs/cluster-api/blob/v1.13.2/internal/controllers/topology/cluster/structuredmerge/dryrun.go#L52-L89
	// https://github.com/kubernetes-sigs/cluster-api/blob/v1.13.2/internal/controllers/topology/cluster/reconcile_state.go#L1296-L1352
	isTopologyDryRun := topology.IsDryRunRequest(req, newObj)
	specChanged := !reflect.DeepEqual(oldObj.Spec.Template.Spec, newObj.Spec.Template.Spec)
	if !isTopologyDryRun && specChanged {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec", "template", "spec"), "ExoscaleMachineTemplate spec.template.spec is immutable"),
		)
	}
	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachineTemplate").GroupKind(),
		newObj.Name,
		allErrs,
	)
}

// ValidateDelete implements admission.Validator.
func (*ExoscaleMachineTemplateCustomValidator) ValidateDelete(_ context.Context, _ *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}
