package domain

import (
	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ClusterValidator is used by validation webhook to validate a cluster and cluster template
// only the package internal/service has access to the default security group rules definition.
type ClusterValidator interface {
	ValidateCreate(spec infrav1alpha1.ExoscaleClusterSpec, fldPath *field.Path) field.ErrorList
	ValidateUpdate(oldSpec, newSpec infrav1alpha1.ExoscaleClusterSpec, fldPath *field.Path) field.ErrorList
}
