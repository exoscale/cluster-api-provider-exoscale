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
	"net"

	"github.com/exoscale/egoscale"
	yaml "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	//ExoscaleIPAnnotationKey represent a machine ip
	ExoscaleIPAnnotationKey = "exoscale-ip-address"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExoscaleMachineProviderStatus is the Schema for the exoscalemachineproviderstatuses API
// +k8s:openapi-gen=true
type ExoscaleMachineProviderStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	ID            *egoscale.UUID `json:"id"`
	IP            net.IP         `json:"ip"`
	SSHKeyName    string         `json:"sshKeyName"`
	SSHPrivateKey string         `json:"sshPrivateKey"`
	TemplateID    *egoscale.UUID `json:"templateID"`
	User          string         `json:"user"`
	ZoneID        *egoscale.UUID `json:"zoneID"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

func init() {
	SchemeBuilder.Register(&ExoscaleMachineProviderStatus{})
}

// MachineStatusFromProviderStatus return machine provider specs from machine provider custom resources (/config/crds)
func MachineStatusFromProviderStatus(providerStatus *runtime.RawExtension) (*ExoscaleMachineProviderStatus, error) {
	config := new(ExoscaleMachineProviderStatus)
	if providerStatus != nil {
		if err := yaml.Unmarshal(providerStatus.Raw, config); err != nil {
			return nil, err
		}
	}
	return config, nil
}
