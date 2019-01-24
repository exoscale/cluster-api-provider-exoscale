package status

import (
	yaml "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
)

//ClusterStatusFromProviderStatus return cluster provider specs from cluster provider custom resources (/config/crds)
func ClusterStatusFromProviderStatus(providerStatus *runtime.RawExtension) (*exoscalev1.ExoscaleClusterProviderStatus, error) {
	config := new(exoscalev1.ExoscaleClusterProviderStatus)
	if providerStatus != nil {
		if err := yaml.Unmarshal(providerStatus.Raw, config); err != nil {
			return nil, err
		}
	}
	return config, nil
}

//MachineSpecFromMachineStatus return machine provider specs from machine provider custom resources (/config/crds)
func MachineSpecFromMachineStatus(providerStatus *runtime.RawExtension) (*exoscalev1.ExoscaleMachineProviderStatus, error) {
	config := new(exoscalev1.ExoscaleMachineProviderStatus)
	if providerStatus != nil {
		if err := yaml.Unmarshal(providerStatus.Raw, config); err != nil {
			return nil, err
		}
	}
	return config, nil
}
