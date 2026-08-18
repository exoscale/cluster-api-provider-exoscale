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

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

func Test_ExoscaleMachineCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	rootVolumeSize := int64(20)
	providerID := "exoscale://11111111-2222-3333-4444-555555555555"
	old := &infrastructurev1alpha1.ExoscaleMachine{
		Spec: infrastructurev1alpha1.ExoscaleMachineSpec{
			Template:          "ubuntu",
			InstanceType:      "small",
			SSHKey:            "default",
			SecurityGroups:    []string{"d1c4e40e-2f26-4dbd-a89a-446c2a9c8114"},
			RootVolumeSizeGiB: &rootVolumeSize,
			ProviderID:        &providerID,
		},
	}

	tests := []struct {
		name    string
		mutate  func(*infrastructurev1alpha1.ExoscaleMachine)
		wantErr bool
	}{
		{
			name:   "unchanged spec",
			mutate: func(*infrastructurev1alpha1.ExoscaleMachine) {},
		},
		{
			name: "provider ID",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				newProviderID := "exoscale://8a991b98-e12e-4ef5-98ea-b44dd829be2f"
				machine.Spec.ProviderID = &newProviderID
			},
		},
		{
			name: "instance type",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				machine.Spec.InstanceType = "medium"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newObj := old.DeepCopy()
			tt.mutate(newObj)

			warnings, err := (&ExoscaleMachineCustomValidator{}).ValidateUpdate(context.Background(), old, newObj)

			assert.Nil(t, warnings)
			if tt.wantErr {
				assert.ErrorContains(t, err, "instance creation fields are immutable")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
