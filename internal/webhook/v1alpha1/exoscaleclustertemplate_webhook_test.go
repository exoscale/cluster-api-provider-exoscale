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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	egoscale "github.com/exoscale/egoscale/v3"
)

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
