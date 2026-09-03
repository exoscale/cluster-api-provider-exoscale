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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
)

func Test_ExoscaleMachineCustomValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	providerID := "exoscale://11111111-2222-3333-4444-555555555555"
	tests := []struct {
		name       string
		providerID *string
		claimed    bool
		wantErr    string
	}{
		{name: "new machine"},
		{name: "claimed new machine", claimed: true, wantErr: "spec.providerID"},
		{name: "provider ID without identity", providerID: &providerID, wantErr: domain.MachineUIDKey},
		{name: "moved machine", providerID: &providerID, claimed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &infrastructurev1alpha1.ExoscaleMachine{
				Spec: infrastructurev1alpha1.ExoscaleMachineSpec{ProviderID: tt.providerID},
			}
			if tt.claimed {
				obj.Annotations = map[string]string{domain.MachineUIDKey: "source-uid"}
			}
			_, err := (&ExoscaleMachineCustomValidator{}).ValidateCreate(context.Background(), obj)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleMachineCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	rootVolumeSize := int64(20)
	providerID := "exoscale://11111111-2222-3333-4444-555555555555"
	old := &infrastructurev1alpha1.ExoscaleMachine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{domain.MachineUIDKey: "source-uid"}},
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
		{
			name: "ownership annotation",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				machine.Annotations[domain.MachineUIDKey] = "other-uid"
			},
			wantErr: true,
		},
		{
			name: "delete for move",
			mutate: func(machine *infrastructurev1alpha1.ExoscaleMachine) {
				machine.Annotations = map[string]string{clusterctlv1.DeleteForMoveAnnotation: ""}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newObj := old.DeepCopy()
			tt.mutate(newObj)

			warnings, err := (&ExoscaleMachineCustomValidator{}).ValidateUpdate(context.Background(), old, newObj)

			assert.Nil(t, warnings)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
