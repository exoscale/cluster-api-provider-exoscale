/*
Copyright 2018 The Kubernetes Authors.
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExoscaleMachineProviderConfig contains Config for Exoscale machines.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ExoscaleMachineProviderConfig struct {
	metav1.TypeMeta `json:",inline"`

	Zone            string `json:"zone,omitempty"`
	Size            string `json:"size,omitempty"`
	RootDiskSize    int64  `json:"rootdisksize,omitempty"`
	ServiceOffering string `json:"serviceoffering,omitempty"`
	Template        string `json:"template,omitempty"`
	KeyPair         string `json:"tags,omitempty"`
	IP6             bool   `json:"ipv6,omitempty"`
}
