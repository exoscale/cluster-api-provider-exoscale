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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

type templateValidationInput struct {
	kind           schema.GroupKind
	name           string
	metadata       clusterv1.ObjectMeta
	specChanged    bool
	topologyDryRun bool
}

func decideTemplateValidation(input templateValidationInput) error {
	allErrs := input.metadata.Validate(field.NewPath("spec", "template", "metadata"))
	if input.specChanged && !input.topologyDryRun {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "template", "spec"),
			input.kind.Kind+".spec.template.spec is immutable, create a new template instead",
		))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(input.kind, input.name, allErrs)
}
