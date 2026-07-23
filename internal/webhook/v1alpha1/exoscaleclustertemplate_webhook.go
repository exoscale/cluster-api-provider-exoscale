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
	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var exoscaleclustertemplatelog = logf.Log.WithName("exoscaleclustertemplate-resource")

// SetupExoscaleClusterTemplateWebhookWithManager registers the webhook for ExoscaleClusterTemplate in the manager.
func SetupExoscaleClusterTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleClusterTemplate{}).
		WithValidator(&ExoscaleClusterTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscaleclustertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclustertemplates,verbs=create;update,versions=v1alpha1,name=vexoscaleclustertemplate-v1alpha1.kb.io,admissionReviewVersions=v1

// ExoscaleClusterTemplateCustomValidator struct is responsible for validating the ExoscaleClusterTemplate resource
// when it is created, updated, or deleted.
type ExoscaleClusterTemplateCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleClusterTemplate.
func (v *ExoscaleClusterTemplateCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1alpha1.ExoscaleClusterTemplate) (admission.Warnings, error) {
	allErrs := obj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleClusterTemplate").GroupKind(),
		obj.Name,
		allErrs,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleClusterTemplate.
func (v *ExoscaleClusterTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleClusterTemplate) (admission.Warnings, error) {
	exoscaleclustertemplatelog.Info("Validation for ExoscaleClusterTemplate upon update", "name", newObj.GetName())

	allErrs := newObj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))
	req, _ := admission.RequestFromContext(ctx)
	if !topology.IsDryRunRequest(req, newObj) && !reflect.DeepEqual(oldObj.Spec.Template.Spec, newObj.Spec.Template.Spec) {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec"),
			"ExoscaleClusterTemplate.spec.template.spec is immutable, create a new template instead",
		))
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleClusterTemplate").GroupKind(),
		newObj.Name,
		allErrs,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleClusterTemplate.
func (v *ExoscaleClusterTemplateCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1alpha1.ExoscaleClusterTemplate) (admission.Warnings, error) {
	return nil, nil
}
