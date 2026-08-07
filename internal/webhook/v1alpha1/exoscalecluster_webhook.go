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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/service"
)

// nolint:unused
var exoscaleclusterlog = logf.Log.WithName("exoscalecluster-resource")

// SetupExoscaleClusterWebhookWithManager registers the webhook for ExoscaleCluster in the manager.
func SetupExoscaleClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1alpha1.ExoscaleCluster{}).
		WithValidator(&ExoscaleClusterCustomValidator{validator: service.NewExoscaleClusterValidator()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-exoscalecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=exoscaleclusters,verbs=create;update,versions=v1alpha1,name=vexoscalecluster-v1alpha1.kb.io,admissionReviewVersions=v1

type ExoscaleClusterCustomValidator struct {
	validator domain.ClusterValidator
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleCluster.
func (v *ExoscaleClusterCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1alpha1.ExoscaleCluster) (admission.Warnings, error) {
	exoscaleclusterlog.Info("Validation for ExoscaleCluster upon creation", "name", obj.GetName())

	if errs := v.validator.ValidateCreate(obj.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1alpha1.SchemeGroupVersion.Group, Kind: "ExoscaleCluster"},
			obj.Name, errs,
		)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleCluster.
func (v *ExoscaleClusterCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1alpha1.ExoscaleCluster) (admission.Warnings, error) {
	exoscaleclusterlog.Info("Validation for ExoscaleCluster upon update", "name", newObj.GetName())

	if errs := v.validator.ValidateUpdate(oldObj.Spec, newObj.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1alpha1.SchemeGroupVersion.Group, Kind: "ExoscaleCluster"},
			newObj.Name, errs,
		)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ExoscaleCluster.
func (v *ExoscaleClusterCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1alpha1.ExoscaleCluster) (admission.Warnings, error) {
	exoscaleclusterlog.Info("Validation for ExoscaleCluster upon deletion", "name", obj.GetName())
	return nil, nil
}
