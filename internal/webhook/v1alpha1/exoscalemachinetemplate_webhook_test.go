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
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

func validExoscaleMachineTemplate() *infrastructurev1alpha1.ExoscaleMachineTemplate {
	return &infrastructurev1alpha1.ExoscaleMachineTemplate{
		Spec: infrastructurev1alpha1.ExoscaleMachineTemplateSpec{
			Template: infrastructurev1alpha1.ExoscaleMachineTemplateResource{
				Spec: infrastructurev1alpha1.ExoscaleMachineSpec{
					Template:     "Linux Ubuntu 24.04 LTS 64-bit",
					InstanceType: "small",
				},
			},
		},
	}
}

func TestExoscaleMachineTemplateCustomValidatorValidateCreate(t *testing.T) {
	t.Parallel()

	validator := ExoscaleMachineTemplateCustomValidator{}
	valid := validExoscaleMachineTemplate()
	_, err := validator.ValidateCreate(context.Background(), valid)
	assert.NoError(t, err)

	invalid := valid.DeepCopy()
	invalid.Spec.Template.ObjectMeta.Labels = map[string]string{"/invalid": "value"}
	_, err = validator.ValidateCreate(context.Background(), invalid)
	assert.ErrorContains(t, err, "spec.template.metadata.labels")

	invalid = valid.DeepCopy()
	invalid.Spec.Template.ObjectMeta.Annotations = map[string]string{"/invalid": "value"}
	_, err = validator.ValidateCreate(context.Background(), invalid)
	assert.ErrorContains(t, err, "spec.template.metadata.annotations")
}

func TestExoscaleMachineTemplateCustomValidatorValidateUpdate(t *testing.T) {
	t.Parallel()

	validator := ExoscaleMachineTemplateCustomValidator{}
	old := validExoscaleMachineTemplate()

	metadataUpdate := old.DeepCopy()
	metadataUpdate.Spec.Template.ObjectMeta = clusterv1.ObjectMeta{
		Labels:      map[string]string{"e2e.cluster.x-k8s.io/mutable": "true"},
		Annotations: map[string]string{"e2e.cluster.x-k8s.io/mutable": "true"},
	}
	_, err := validator.ValidateUpdate(context.Background(), old, metadataUpdate)
	assert.NoError(t, err)

	invalidMetadata := old.DeepCopy()
	invalidMetadata.Spec.Template.ObjectMeta.Labels = map[string]string{"/invalid": "value"}
	_, err = validator.ValidateUpdate(context.Background(), old, invalidMetadata)
	assert.ErrorContains(t, err, "spec.template.metadata.labels")

	specUpdate := old.DeepCopy()
	specUpdate.Spec.Template.Spec.InstanceType = "medium"
	_, err = validator.ValidateUpdate(context.Background(), old, specUpdate)
	assert.ErrorContains(t, err, "ExoscaleMachineTemplate.spec.template.spec is immutable")

	dryRun := true
	specUpdate.Annotations = map[string]string{clusterv1.TopologyDryRunAnnotation: ""}
	ctx := admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: &dryRun}})
	_, err = validator.ValidateUpdate(ctx, old, specUpdate)
	assert.NoError(t, err)
}
