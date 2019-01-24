package specs

import (
	yaml "gopkg.in/yaml.v2"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

//ClusterSpecFromProviderSpec return cluster provider specs (e.g cluster.yml)
func ClusterSpecFromProviderSpec(providerConfig clusterv1.ProviderSpec) (*exoscalev1.ExoscaleClusterProviderSpec, error) {
	config := new(exoscalev1.ExoscaleClusterProviderSpec)
	if err := yaml.Unmarshal(providerConfig.Value.Raw, config); err != nil {
		return nil, err
	}
	return config, nil
}

//MachineSpecFromProviderSpec return machine provider specs (e.g machine.yml)
func MachineSpecFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*exoscalev1.ExoscaleMachineProviderSpec, error) {
	config := new(exoscalev1.ExoscaleMachineProviderSpec)
	if err := yaml.Unmarshal(providerSpec.Value.Raw, config); err != nil {
		return nil, err
	}
	return config, nil
}
