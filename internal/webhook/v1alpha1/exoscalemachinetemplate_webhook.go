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

	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

var exoscalemachinetemplatelog = logf.Log.WithName("exoscalemachinetemplate-resource")

// SetupExoscaleMachineTemplateWebhookWithManager registers the webhook for ExoscaleMachineTemplate in the manager.
func SetupExoscaleMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleMachineTemplate{}).
		WithValidator(&ExoscaleMachineTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscalemachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscalemachinetemplates,verbs=create;update,versions=v1alpha1,name=vexoscalemachinetemplate-v1alpha1.kb.io,admissionReviewVersions=v1

// ExoscaleMachineTemplateCustomValidator validates ExoscaleMachineTemplate resources.
type ExoscaleMachineTemplateCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator.
func (v *ExoscaleMachineTemplateCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	return nil, decideTemplateValidation(templateValidationInput{
		kind:     infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachineTemplate").GroupKind(),
		name:     obj.Name,
		metadata: obj.Spec.Template.ObjectMeta,
	})
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *ExoscaleMachineTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	exoscalemachinetemplatelog.Info("Validation for ExoscaleMachineTemplate upon update", "name", newObj.GetName())

	req, _ := admission.RequestFromContext(ctx)
	return nil, decideTemplateValidation(templateValidationInput{
		kind:           infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachineTemplate").GroupKind(),
		name:           newObj.Name,
		metadata:       newObj.Spec.Template.ObjectMeta,
		specChanged:    !reflect.DeepEqual(oldObj.Spec.Template.Spec, newObj.Spec.Template.Spec),
		topologyDryRun: topology.IsDryRunRequest(req, newObj),
	})
}

// ValidateDelete implements webhook.CustomValidator.
func (v *ExoscaleMachineTemplateCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1alpha1.ExoscaleMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}
