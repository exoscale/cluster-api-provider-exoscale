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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	infrastructurev1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/mocks"
)

func Test_ExoscaleClusterCustomValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	obj := &infrastructurev1alpha1.ExoscaleCluster{ObjectMeta: v1.ObjectMeta{Name: "my-cluster"}}

	tests := []struct {
		name      string
		validator func(m *mocks.ClusterValidator)
		err       error
	}{
		{
			name: "valid spec admits creation",
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateCreate(obj.Spec, field.NewPath("spec")).
					Return(nil)
			},
		},
		{
			name: "invalid spec is rejected",
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateCreate(obj.Spec, field.NewPath("spec")).
					Return(field.ErrorList{field.Forbidden(field.NewPath("spec", "zone"), assert.AnError.Error())})
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			t.Parallel()

			validatorMock := mocks.NewClusterValidator(t)
			if ut.validator != nil {
				ut.validator(validatorMock)
			}

			validator := ExoscaleClusterCustomValidator{validator: validatorMock}
			warning, err := validator.ValidateCreate(context.Background(), obj)

			assert.Nil(t, warning)
			if ut.err != nil {
				assert.ErrorContains(t, err, ut.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleClusterCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	oldObj := &infrastructurev1alpha1.ExoscaleCluster{}
	newObj := &infrastructurev1alpha1.ExoscaleCluster{ObjectMeta: v1.ObjectMeta{Name: "my-cluster"}}

	tests := []struct {
		name      string
		validator func(m *mocks.ClusterValidator)
		err       error
	}{
		{
			name: "valid spec admits the update",
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateUpdate(oldObj.Spec, newObj.Spec, field.NewPath("spec")).
					Return(nil)
			},
		},
		{
			name: "invalid spec is rejected",
			validator: func(m *mocks.ClusterValidator) {
				m.EXPECT().
					ValidateUpdate(oldObj.Spec, newObj.Spec, field.NewPath("spec")).
					Return(field.ErrorList{field.Forbidden(field.NewPath("spec", "zone"), assert.AnError.Error())})
			},
			err: assert.AnError,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			t.Parallel()

			validatorMock := mocks.NewClusterValidator(t)
			if ut.validator != nil {
				ut.validator(validatorMock)
			}

			validator := ExoscaleClusterCustomValidator{validator: validatorMock}
			warning, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)

			assert.Nil(t, warning)
			if ut.err != nil {
				assert.ErrorContains(t, err, ut.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ExoscaleClusterCustomDefaulter_Default(t *testing.T) {
	t.Parallel()

	const existingID = "3f2a1b4c-5d6e-4f70-8192-a3b4c5d6e7f8"

	tests := []struct {
		name      string
		clusterID string
		expected  string
	}{
		{
			name:     "assigns an identity when the spec carries none",
			expected: "",
		},
		{
			name:      "keeps the identity the spec already carries",
			clusterID: existingID,
			expected:  existingID,
		},
	}

	for _, ut := range tests {
		t.Run(ut.name, func(t *testing.T) {
			t.Parallel()

			obj := &infrastructurev1alpha1.ExoscaleCluster{
				ObjectMeta: v1.ObjectMeta{Name: "my-cluster"},
				Spec:       infrastructurev1alpha1.ExoscaleClusterSpec{ClusterID: ut.clusterID},
			}

			assert.NoError(t, (&ExoscaleClusterCustomDefaulter{}).Default(context.Background(), obj))

			if ut.expected != "" {
				assert.Equal(t, ut.expected, obj.Spec.ClusterID)
				return
			}

			_, err := uuid.Parse(obj.Spec.ClusterID)
			assert.NoError(t, err, "defaulter must assign a valid UUID")
		})
	}
}
