/*
Copyright 2019 The Kubernetes Authors.

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
	yaml "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleClusterProviderSpec is the Schema for the exoscaleclusterproviderspecs API
// +k8s:openapi-gen=true
type ExoscaleClusterProviderSpec struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// SecurityGroup is the name of firewalling security group.
	SecurityGroup string `json:"securityGroup"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

func init() {
	SchemeBuilder.Register(&ExoscaleClusterProviderSpec{})
}

//ClusterSpecFromProviderSpec return cluster provider specs (e.g cluster.yml)
func ClusterSpecFromProviderSpec(providerConfig clusterv1.ProviderSpec) (*ExoscaleClusterProviderSpec, error) {
	config := new(ExoscaleClusterProviderSpec)
	if err := yaml.Unmarshal(providerConfig.Value.Raw, config); err != nil {
		return nil, err
	}
	return config, nil
}
