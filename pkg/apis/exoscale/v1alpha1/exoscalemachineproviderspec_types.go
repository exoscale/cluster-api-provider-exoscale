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

// ExoscaleMachineProviderSpec is the Schema for the exoscalemachineproviderconfigs API
// +k8s:openapi-gen=true
type ExoscaleMachineProviderSpec struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	AntiAffinityGroup string `json:"antiAffinityGroup,omitempty"`
	CloudInit         string `json:"cloudInit,omitempty"`
	Disk              int64  `json:"disk"`
	IPv6              bool   `json:"ipv6,omitempty"`
	SSHKey            string `json:"sshKey"`
	Template          string `json:"template"`
	Provisioned       bool   `json:"provisioned"`
	Type              string `json:"type"`
	Zone              string `json:"zone"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

func init() {
	SchemeBuilder.Register(&ExoscaleMachineProviderSpec{})
}

//MachineSpecFromProviderSpec return machine provider specs (e.g machine.yml)
func MachineSpecFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*ExoscaleMachineProviderSpec, error) {
	config := new(ExoscaleMachineProviderSpec)
	if err := yaml.Unmarshal(providerSpec.Value.Raw, config); err != nil {
		return nil, err
	}
	return config, nil
}
