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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

func Test_ExoscaleMachineCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	rootVolumeSize := int64(20)
	providerID := "exoscale://11111111-2222-3333-4444-555555555555"
	const machineID = "3f2a1b4c-5d6e-4f70-8192-a3b4c5d6e7f8"
	old := &infrastructurev1alpha1.ExoscaleMachine{
		Spec: infrastructurev1alpha1.ExoscaleMachineSpec{
			MachineID:         machineID,
			Template:          "ubuntu",
			InstanceType:      "small",
			SSHKey:            "default",
			SecurityGroups:    []string{"d1c4e40e-2f26-4dbd-a89a-446c2a9c8114"},
			RootVolumeSizeGiB: &rootVolumeSize,
			ProviderID:        &providerID,
		},
	}

	tests := []struct {
		name      string
		mutateOld func(*infrastructurev1alpha1.ExoscaleMachine)
		mutate    func(*infrastructurev1alpha1.ExoscaleMachine)
		wantErr   string
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
			wantErr: "instance creation fields are immutable",
		},
		{
			// The controller assigns it on the first reconciliation when the defaulting
			// webhook is not running, as with ENABLE_WEBHOOKS=false.
			name:      "machine ID assigned when it had none",
			mutateOld: func(machine *infrastructurev1alpha1.ExoscaleMachine) { machine.Spec.MachineID = "" },
			mutate:    func(*infrastructurev1alpha1.ExoscaleMachine) {},
		},
		{
			name: "machine ID changed",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				machine.Spec.MachineID = "0a1b2c3d-4e5f-4061-8273-8495a6b7c8d9"
			},
			wantErr: "machineID cannot be changed or removed once set",
		},
		{
			// Losing the identity strands the instance provisioned for this machine.
			name: "machine ID removed",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				machine.Spec.MachineID = ""
			},
			wantErr: "machineID cannot be changed or removed once set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldObj := old.DeepCopy()
			if tt.mutateOld != nil {
				tt.mutateOld(oldObj)
			}
			newObj := old.DeepCopy()
			tt.mutate(newObj)

			warnings, err := (&ExoscaleMachineCustomValidator{}).ValidateUpdate(context.Background(), oldObj, newObj)

			assert.Nil(t, warnings)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleMachineCustomDefaulter_Default(t *testing.T) {
	t.Parallel()

	const existingID = "3f2a1b4c-5d6e-4f70-8192-a3b4c5d6e7f8"

	tests := []struct {
		name      string
		machineID string
		expected  string
	}{
		{
			name: "assigns an identity when the spec carries none",
		},
		{
			// `clusterctl move` re-creates the resource in the target management cluster with
			// the spec it copied over: regenerating the identity here would strand the running
			// instance and provision a second one.
			name:      "keeps the identity the spec already carries",
			machineID: existingID,
			expected:  existingID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			obj := &infrastructurev1alpha1.ExoscaleMachine{
				Spec: infrastructurev1alpha1.ExoscaleMachineSpec{MachineID: tt.machineID},
			}

			assert.NoError(t, (&ExoscaleMachineCustomDefaulter{}).Default(context.Background(), obj))

			if tt.expected != "" {
				assert.Equal(t, tt.expected, obj.Spec.MachineID)
				return
			}

			_, err := uuid.Parse(obj.Spec.MachineID)
			assert.NoError(t, err, "defaulter must assign a valid UUID")
		})
	}
}
