/*
Copyright 2018 The Kubernetes authors.

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

package machine

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/exoscale/egoscale"
	"k8s.io/klog"

	yaml "gopkg.in/yaml.v2"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exoclient "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/client"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
	//ProviderName Exoscale provider name
	ProviderName            = "exoscale"
	ExoscaleIPAnnotationKey = "exoscale-ip"
)

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	machinesGetter client.MachinesGetter
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	MachinesGetter client.MachinesGetter
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		machinesGetter: params.MachinesGetter,
	}, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	log.Printf("Creating machine %v for cluster %v.", machine.Name, cluster.Name)

	providerSpec, err := machineSpecFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal providerSpec field: %v", err)
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	//Prerequisite
	// create or upload an sshkey in exoscale
	// put sshkey name in machine spec provider yml

	z, err := exoClient.GetWithContext(ctx, &egoscale.Zone{Name: providerSpec.Zone})
	if err != nil {
		return fmt.Errorf("Invalid exoscale zone %q. providerSpec field: %v", providerSpec.Zone, err)
	}
	zone := z.(*egoscale.Zone)

	t, err := exoClient.GetWithContext(
		ctx,
		&egoscale.Template{
			Name:   providerSpec.Template,
			ZoneID: zone.ID,
		},
	)
	if err != nil {
		return fmt.Errorf("Invalid exoscale template %q. providerSpec field: %v", providerSpec.Zone, err)
	}
	template := t.(*egoscale.Template)

	sg, err := exoClient.GetWithContext(
		ctx,
		&egoscale.SecurityGroup{
			Name: providerSpec.SecurityGroup,
		},
	)
	if err != nil {
		return fmt.Errorf("Invalid exoscale security-group %q. providerSpec field: %v", providerSpec.Zone, err)
	}
	securityGroup := sg.(*egoscale.SecurityGroup)

	so, err := exoClient.GetWithContext(
		ctx,
		&egoscale.ServiceOffering{
			Name: providerSpec.Type,
		},
	)
	if err != nil {
		return fmt.Errorf("Invalid exoscale service-Offering %q. providerSpec field: %v", providerSpec.Zone, err)
	}
	serviceOffering := so.(*egoscale.ServiceOffering)

	req := egoscale.DeployVirtualMachine{
		Name: machine.Name,
		//UserData:          userData,
		ZoneID:           zone.ID,
		TemplateID:       template.ID,
		RootDiskSize:     int64(providerSpec.Disk),
		KeyPair:          providerSpec.SSHKey,
		SecurityGroupIDs: []egoscale.UUID{*securityGroup.ID},
		IP6:              &providerSpec.Ipv6,
		//NetworkIDs:        pvs,
		ServiceOfferingID: serviceOffering.ID,
		//AffinityGroupIDs:  affinitygroups,
	}

	resp, err := exoClient.RequestWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("exoscale failed to DeployVirtualMachine %v", err)
	}

	vm := resp.(*egoscale.VirtualMachine)

	log.Println("Deployed instance:", vm.Name, "IP:", vm.IP().String())

	machine.Annotations["exoscale-ip"] = vm.IP().String()

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	log.Printf("Deleting machine %v for cluster %v.", machine.Name, cluster.Name)

	return fmt.Errorf("TODO: Not yet implemented")
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	log.Printf("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	log.Printf("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
	return false, fmt.Errorf("TODO: Not yet implemented")
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (*Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	log.Printf("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)
	if machine.ObjectMeta.Annotations != nil {
		if ip, ok := machine.ObjectMeta.Annotations[ExoscaleIPAnnotationKey]; ok {
			klog.Infof("Returning IP from machine annotation %s", ip)
			return ip, nil
		}
	}

	return "", errors.New("could not get IP")
}

// GetKubeConfig gets a kubeconfig from the master.
func (*Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	log.Printf("Getting IP of machine %v for cluster %v.", master.Name, cluster.Name)
	return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() not yet implemented")
}

func machineSpecFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*exoscalev1.ExoscaleMachineProviderSpecSpec, error) {
	if providerSpec.Value == nil {
		return nil, errors.New("no such providerSpec found in manifest")
	}

	var config exoscalev1.ExoscaleMachineProviderSpecSpec
	if err := yaml.Unmarshal(providerSpec.Value.Raw, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
