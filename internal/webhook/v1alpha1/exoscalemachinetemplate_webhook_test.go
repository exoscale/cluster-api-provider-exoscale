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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
)

func Test_ExoscaleMachineTemplateCustomValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*infrastructurev1alpha1.ExoscaleMachineTemplate)
		wantErr string
	}{
		{
			name:   "valid metadata",
			mutate: func(*infrastructurev1alpha1.ExoscaleMachineTemplate) {},
		},
		{
			name: "invalid metadata",
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.ObjectMeta.Labels = map[string]string{"invalid key": "value"}
			},
			wantErr: "spec.template.metadata.labels",
		},
		{
			name: "reserved ownership annotation",
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.ObjectMeta.Annotations = map[string]string{domain.MachineUIDKey: "shared"}
			},
			wantErr: domain.MachineUIDKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validExoscaleMachineTemplate()
			tt.mutate(obj)

			warnings, err := (&ExoscaleMachineTemplateCustomValidator{}).ValidateCreate(context.Background(), obj)

			assert.Nil(t, warnings)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleMachineTemplateCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()
	const updatedInstanceType = "medium"

	tests := []struct {
		name    string
		dryRun  bool
		mutate  func(*infrastructurev1alpha1.ExoscaleMachineTemplate)
		wantErr string
	}{
		{
			name:   "unchanged template",
			mutate: func(*infrastructurev1alpha1.ExoscaleMachineTemplate) {},
		},
		{
			name: "template metadata",
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.ObjectMeta.Labels["role"] = "control-plane"
			},
		},
		{
			name: "machine spec",
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.Spec.InstanceType = updatedInstanceType
			},
			wantErr: "spec.template.spec is immutable",
		},
		{
			name:   "topology dry-run",
			dryRun: true,
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Annotations = map[string]string{clusterv1.TopologyDryRunAnnotation: ""}
				template.Spec.Template.Spec.InstanceType = updatedInstanceType
			},
		},
		{
			name:   "unannotated dry-run",
			dryRun: true,
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.Spec.InstanceType = updatedInstanceType
			},
			wantErr: "spec.template.spec is immutable",
		},
		{
			name: "invalid metadata",
			mutate: func(template *infrastructurev1alpha1.ExoscaleMachineTemplate) {
				template.Spec.Template.ObjectMeta.Labels = map[string]string{"invalid key": "value"}
			},
			wantErr: "spec.template.metadata.labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldObj := validExoscaleMachineTemplate()
			newObj := oldObj.DeepCopy()
			tt.mutate(newObj)
			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{DryRun: new(tt.dryRun)},
			})

			warnings, err := (&ExoscaleMachineTemplateCustomValidator{}).ValidateUpdate(ctx, oldObj, newObj)

			assert.Nil(t, warnings)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func validExoscaleMachineTemplate() *infrastructurev1alpha1.ExoscaleMachineTemplate {
	return &infrastructurev1alpha1.ExoscaleMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "machine-template"},
		Spec: infrastructurev1alpha1.ExoscaleMachineTemplateSpec{
			Template: infrastructurev1alpha1.ExoscaleMachineTemplateResource{
				ObjectMeta: clusterv1.ObjectMeta{Labels: map[string]string{"role": "worker"}},
				Spec: infrastructurev1alpha1.ExoscaleMachineSpec{
					Template:     "ubuntu",
					InstanceType: "small",
				},
			},
		},
	}
}
