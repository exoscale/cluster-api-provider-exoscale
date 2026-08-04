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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
	egoscale "github.com/exoscale/egoscale/v3"
)

func Test_ExoscaleClusterTemplateCustomValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	obj := &infrastructurev1alpha1.ExoscaleClusterTemplate{ObjectMeta: v1.ObjectMeta{Name: "my-template"}}

	invalidMetadataObj := &infrastructurev1alpha1.ExoscaleClusterTemplate{
		ObjectMeta: v1.ObjectMeta{Name: "my-template"},
		Spec: infrastructurev1alpha1.ExoscaleClusterTemplateSpec{
			Template: infrastructurev1alpha1.ExoscaleClusterTemplateResource{
				ObjectMeta: v1beta2.ObjectMeta{
					Labels: map[string]string{"invalid label key": "value"},
				},
			},
		},
	}

	tests := []struct {
		name      string
		obj       *infrastructurev1alpha1.ExoscaleClusterTemplate
		validator func(m *mocks.ClusterValidator)
		err       error
	}{
		{
			name: "nominal",
			obj:  obj,
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateCreate(obj.Spec.Template.Spec, field.NewPath("spec", "template", "spec")).
					Return(nil)
			},
		},
		{
			name: "validate create returns an error",
			obj:  obj,
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateCreate(obj.Spec.Template.Spec, field.NewPath("spec", "template", "spec")).
					Return(field.ErrorList{field.Forbidden(field.NewPath("spec", "template", "spec", "zone"), assert.AnError.Error())})
			},
			err: assert.AnError,
		},
		{
			name: "invalid template metadata is rejected",
			obj:  invalidMetadataObj,
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateCreate(invalidMetadataObj.Spec.Template.Spec, field.NewPath("spec", "template", "spec")).
					Return(nil)
			},
			err: errors.New("invalid label key"),
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			t.Parallel()

			validatorMock := mocks.NewClusterValidator(t)
			if ut.validator != nil {
				ut.validator(validatorMock)
			}

			validator := ExoscaleClusterTemplateCustomValidator{validator: validatorMock}
			warning, err := validator.ValidateCreate(context.Background(), ut.obj)

			assert.Nil(t, warning)
			if ut.err != nil {
				assert.ErrorContains(t, err, ut.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleClusterTemplateCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	old := &infrastructurev1alpha1.ExoscaleClusterTemplate{
		Spec: infrastructurev1alpha1.ExoscaleClusterTemplateSpec{
			Template: infrastructurev1alpha1.ExoscaleClusterTemplateResource{
				Spec: infrastructurev1alpha1.ExoscaleClusterSpec{
					Zone:                 egoscale.ZoneNameCHDk2,
					ControlPlaneEndpoint: infrastructurev1alpha1.APIEndpoint{Port: 12345},
					ExoscaleSecret:       infrastructurev1alpha1.ExoscaleSecretRef{Name: "ch-gva-2"},
				},
			},
		},
	}

	tests := []struct {
		name    string
		input   *infrastructurev1alpha1.ExoscaleClusterTemplate
		warning admission.Warnings
		err     error
	}{
		{
			name:  "nominal",
			input: old,
		},
		{
			name: "nominal",
			input: func() *infrastructurev1alpha1.ExoscaleClusterTemplate {
				v := *old
				v.Spec.Template.Spec.ControlPlaneEndpoint.Port = 54321
				return &v
			}(),
			err: errors.New("ExoscaleClusterTemplate.spec is immutable, create a new template instead"),
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			validator := ExoscaleClusterTemplateCustomValidator{}
			warning, err := validator.ValidateUpdate(context.Background(), old, ut.input)

			assert.Equal(t, ut.warning, warning)
			if ut.err != nil {
				assert.ErrorContains(t, err, ut.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
