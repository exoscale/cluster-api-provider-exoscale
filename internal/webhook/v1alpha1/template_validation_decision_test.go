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
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

func TestDecideTemplateValidation(t *testing.T) {
	t.Parallel()

	kind := infrastructurev1alpha1.GroupVersion.WithKind("ExoscaleMachineTemplate").GroupKind()
	invalidMetadata := clusterv1.ObjectMeta{Labels: map[string]string{"/invalid": "value"}}
	tests := []struct {
		name      string
		input     templateValidationInput
		wantError string
	}{
		{name: "valid", input: templateValidationInput{kind: kind, name: "template"}},
		{name: "invalid metadata", input: templateValidationInput{kind: kind, name: "template", metadata: invalidMetadata}, wantError: "spec.template.metadata.labels"},
		{name: "changed spec", input: templateValidationInput{kind: kind, name: "template", specChanged: true}, wantError: "ExoscaleMachineTemplate.spec.template.spec is immutable, create a new template instead"},
		{name: "topology dry-run permits changed spec", input: templateValidationInput{kind: kind, name: "template", specChanged: true, topologyDryRun: true}},
		{name: "topology dry-run still validates metadata", input: templateValidationInput{kind: kind, name: "template", metadata: invalidMetadata, specChanged: true, topologyDryRun: true}, wantError: "spec.template.metadata.labels"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := decideTemplateValidation(tc.input)
			if tc.wantError == "" {
				assert.NoError(t, err)
				return
			}
			if !assert.ErrorContains(t, err, tc.wantError) {
				return
			}
			assert.True(t, apierrors.IsInvalid(err))
			status := err.(apierrors.APIStatus).Status()
			assert.Equal(t, kind.Group, status.Details.Group)
			assert.Equal(t, kind.Kind, status.Details.Kind)
			assert.Equal(t, "template", status.Details.Name)
		})
	}
}
