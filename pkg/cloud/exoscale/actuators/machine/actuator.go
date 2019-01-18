/*
Copyright 2019 The Kubernetes authors.

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

	"github.com/exoscale/egoscale"
	"k8s.io/klog"

	"github.com/ghodss/yaml"
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
	klog.Infof("Creating machine %v for cluster %v.", machine.Name, cluster.Name)

	clusterStatus, err := clusterStatusFromClusterStatus(cluster.Status)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal cluster.Status field: %v", err)
	}

	machineConfig, err := machineSpecFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	//Prerequisite
	// create or upload an sshkey in exoscale
	// put sshkey name in machine spec provider yml

	klog.Warningf("clusterStatus: %#v", clusterStatus)
	klog.Warningf("machineConfig: %#v", machineConfig.Spec)

	z, err := exoClient.GetWithContext(ctx, &egoscale.Zone{Name: machineConfig.Spec.Zone})
	if err != nil {
		return fmt.Errorf("problem fetching the zone %q. %s", machineConfig.Spec.Zone, err)
	}
	zone := z.(*egoscale.Zone)

	t, err := exoClient.GetWithContext(
		ctx,
		&egoscale.Template{
			Name:       machineConfig.Spec.Template,
			ZoneID:     zone.ID,
			IsFeatured: true,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching the template %q. %s", machineConfig.Spec.Template, err)
	}
	template := t.(*egoscale.Template)

	so, err := exoClient.GetWithContext(
		ctx,
		&egoscale.ServiceOffering{
			Name: machineConfig.Spec.Type,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching service-offering %q. %s", machineConfig.Spec.Type, err)
	}
	serviceOffering := so.(*egoscale.ServiceOffering)

	sshKeyName := machine.Name + "id_rsa"

	keyPairs, err := createSSHKey(ctx, exoClient, sshKeyName)
	if err != nil {
		r := err.(*egoscale.ErrorResponse)
		if r.ErrorCode != egoscale.ParamError && r.CSErrorCode != egoscale.InvalidParameterValueException {
			return err
		}
		return fmt.Errorf("an SSH key with that name %q already exists, please choose a different name", sshKeyName)
	}

	securityGroupID, err := egoscale.ParseUUID(clusterStatus.SecurityGroupID)
	if err != nil {
		return fmt.Errorf("invalid securityGroupID field %s. %s", clusterStatus.SecurityGroupID, err)
	}

	req := egoscale.DeployVirtualMachine{
		Name:              machine.Name,
		ZoneID:            zone.ID,
		TemplateID:        template.ID,
		RootDiskSize:      machineConfig.Spec.Disk,
		KeyPair:           sshKeyName,
		SecurityGroupIDs:  []egoscale.UUID{*securityGroupID},
		ServiceOfferingID: serviceOffering.ID,
	}

	resp, err := exoClient.RequestWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("exoscale failed to DeployVirtualMachine %v", err)
	}

	vm := resp.(*egoscale.VirtualMachine)

	klog.Infof("Deployed instance:", vm.Name, "IP:", vm.IP().String())

	klog.Infof("Bootstrapping Kubernetes cluster (can take up to several minutes):")

	sshClient, err := newSSHClient(
		vm.IP().String(),
		"ubuntu",
		keyPairs.PrivateKey,
	)
	if err != nil {
		return fmt.Errorf("unable to initialize SSH client: %s", err)
	}

	if err := bootstrapExokubeCluster(sshClient, kubeCluster{
		Name:              cluster.Name,
		KubernetesVersion: "1.12.5",
		CalicoVersion:     kubeCalicoVersion,
		DockerVersion:     kubeDockerVersion,
		Address:           vm.IP().String(),
	}, false); err != nil {
		return fmt.Errorf("cluster bootstrap failed: %s", err)
	}

	klog.Infof("Machine %q provisioning success!", machine.Name)

	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}
	machine.Annotations["exoscale-ip"] = vm.IP().String()

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	klog.Infof("Deleting machine %v for cluster %v.", machine.Name, cluster.Name)

	klog.Error("Deleting a machine is not yet implemented")
	return nil
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	klog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)

	fmt.Printf("VVVVVVVV=%#v=VVVVVVVVVVV\n", machine.Status)

	klog.Error("Updating a machine is not yet implemented")
	return nil
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	klog.Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)

	exoClient, err := exoclient.Client()
	if err != nil {
		return false, err
	}

	vms, err := exoClient.ListWithContext(ctx, &egoscale.VirtualMachine{Name: machine.Name})
	if err != nil {
		return false, err
	}

	if len(vms) == 1 {
		return true, nil
	}

	if len(vms) > 1 {
		return false, fmt.Errorf("Machine.Exist more than one machine found with this name %s", machine.Name)
	}

	return false, nil
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (*Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	klog.Infof("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)

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
	klog.Infof("Getting IP of machine %v for cluster %v.", master.Name, cluster.Name)

	return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() not yet implemented")
}

func clusterSpecFromProviderSpec(providerConfig clusterv1.ProviderSpec) (*exoscalev1.ExoscaleClusterProviderSpec, error) {
	config := new(exoscalev1.ExoscaleClusterProviderSpec)
	if err := yaml.Unmarshal(providerConfig.Value.Raw, config); err != nil {
		return nil, err
	}
	return config, nil
}

func clusterStatusFromClusterStatus(clusterStatus clusterv1.ClusterStatus) (*exoscalev1.ExoscaleClusterProviderStatus, error) {
	config := new(exoscalev1.ExoscaleClusterProviderStatus)
	if clusterStatus.ProviderStatus != nil {
		if err := yaml.Unmarshal(clusterStatus.ProviderStatus.Raw, config); err != nil {
			return nil, err
		}
	}
	return config, nil
}

func machineSpecFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*exoscalev1.ExoscaleMachineProviderSpec, error) {
	if providerSpec.Value == nil {
		return nil, errors.New("no such providerConfig found in manifest")
	}

	var config exoscalev1.ExoscaleMachineProviderSpec
	if err := yaml.Unmarshal(providerSpec.Value.Raw, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
