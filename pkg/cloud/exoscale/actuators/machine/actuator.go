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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/exoscale/egoscale"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exossh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"
	exoclient "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/client"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
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

	clusterStatus, err := exoscalev1.ClusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal cluster.Status field: %v", err)
	}

	machineConfig, err := exoscalev1.MachineSpecFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	machineStatus, err := exoscalev1.MachineSpecFromMachineStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	if clusterStatus.SecurityGroupID == nil {
		return fmt.Errorf("empty cluster securityGroupID field. %#v", clusterStatus)
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	z, err := exoClient.GetWithContext(ctx, &egoscale.Zone{Name: machineConfig.Zone})
	if err != nil {
		return fmt.Errorf("problem fetching the zone %q. %s", machineConfig.Zone, err)
	}
	zone := z.(*egoscale.Zone)

	t, err := exoClient.GetWithContext(
		ctx,
		&egoscale.Template{
			Name:       machineConfig.Template,
			ZoneID:     zone.ID,
			IsFeatured: true,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching the template %q. %s", machineConfig.Template, err)
	}
	template := t.(*egoscale.Template)
	username, ok := template.Details["username"]
	if !ok {
		return fmt.Errorf("problem fetching username for template %q", template.Name)
	}

	so, err := exoClient.GetWithContext(
		ctx,
		&egoscale.ServiceOffering{
			Name: machineConfig.Type,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching service-offering %q. %s", machineConfig.Type, err)
	}
	serviceOffering := so.(*egoscale.ServiceOffering)

	sshKeyName := machine.Name + "_id_rsa"

	keyPairs, err := exossh.CreateSSHKey(ctx, exoClient, sshKeyName)
	if err != nil {
		r := err.(*egoscale.ErrorResponse)
		if r.ErrorCode != egoscale.ParamError && r.CSErrorCode != egoscale.InvalidParameterValueException {
			return err
		}
		return fmt.Errorf("an SSH key with that name %q already exists, please choose a different name", sshKeyName)
	}

	req := egoscale.DeployVirtualMachine{
		Name:              machine.Name,
		ZoneID:            zone.ID,
		TemplateID:        template.ID,
		RootDiskSize:      machineConfig.Disk,
		KeyPair:           sshKeyName,
		SecurityGroupIDs:  []egoscale.UUID{*clusterStatus.SecurityGroupID},
		ServiceOfferingID: serviceOffering.ID,
	}

	resp, err := exoClient.RequestWithContext(ctx, req)
	if err != nil {
		cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("exoscale failed to DeployVirtualMachine %v", err)
	}

	vm := resp.(*egoscale.VirtualMachine)

	klog.Infof("Deployed instance: %q, IP: %s, password: %q", vm.Name, vm.IP().String(), vm.Password)

	klog.Infof("Bootstrapping Kubernetes cluster (can take up to several minutes):")

	sshClient, err := exossh.NewSSHClient(
		vm.IP().String(),
		username,
		keyPairs.PrivateKey,
	)
	if err != nil {
		cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("unable to initialize SSH client: %s", err)
	}

	// XXX KubernetesVersion should be coming from the MachineSpec
	// https://godoc.org/sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1#MachineVersionInfo
	if err := bootstrapExokubeCluster(sshClient, kubeCluster{
		Name:              cluster.Name,
		KubernetesVersion: "1.12.5",
		CalicoVersion:     kubeCalicoVersion,
		DockerVersion:     kubeDockerVersion,
		Address:           vm.IP().String(),
	}, false); err != nil {
		cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("cluster bootstrap failed: %s", err)
	}

	klog.Infof("Machine %q provisioning success!", machine.Name)

	machineStatus = &exoscalev1.ExoscaleMachineProviderStatus{
		metav1.TypeMeta{
			Kind:       "ExoscaleMachineProviderStatus",
			APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
		},
		metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{time.Now()},
		},
		vm.ID,
		*vm.IP(),
		keyPairs.Name,
		keyPairs.PrivateKey,
		vm.TemplateID,
		username,
		vm.ZoneID,
	}

	if err := a.updateResources(machineStatus, machine); err != nil {
		cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("failed to update machine resources: %s", err)
	}

	// XXX annotations should be replaced by the proper NodeRef
	// https://github.com/kubernetes-sigs/cluster-api/blob/3b5183805f4dbf859d39a2600b268192a8191950/cmd/clusterctl/clusterdeployer/clusterclient/clusterclient.go#L579-L581
	annotations := machine.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[exoscalev1.ExoscaleIPAnnotationKey] = vm.IP().String()
	machine.SetAnnotations(annotations)

	machineClient := a.machinesGetter.Machines(machine.Namespace)
	if _, err := machineClient.Update(machine); err != nil {
		return err
	}

	return nil
}

func (a *Actuator) updateResources(machineStatus *exoscalev1.ExoscaleMachineProviderStatus, machine *clusterv1.Machine) error {
	rawStatus, err := json.Marshal(machineStatus)
	if err != nil {
		return err
	}

	machine.Status.ProviderStatus = &runtime.RawExtension{
		Raw: rawStatus,
	}

	machineClient := a.machinesGetter.Machines(machine.Namespace)

	if _, err := machineClient.UpdateStatus(machine); err != nil {
		return err
	}

	return nil
}

func cleanSSHKey(exoClient *egoscale.Client, sshKeyName string) {
	_ = exoClient.Delete(egoscale.SSHKeyPair{Name: sshKeyName})
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if cluster == nil {
		klog.Warningf("cluster %q as been removed already", machine.Name)
		klog.Infof("deleting machine %q.", machine.Name)
	} else {
		klog.Infof("deleting machine %q from %q.", machine.Name, cluster.Name)
	}

	machineStatus, err := exoscalev1.MachineSpecFromMachineStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("cannot unmarshal machine.Spec field: %v", err)
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	if err := exoClient.Delete(egoscale.SSHKeyPair{Name: machineStatus.SSHKeyName}); err != nil {
		return fmt.Errorf("cannot delete machine SSH KEY: %v", err)
	}

	return exoClient.Delete(egoscale.VirtualMachine{
		ID: machineStatus.ID,
	})
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	klog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)

	machineStatus, err := exoscalev1.MachineSpecFromMachineStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	fmt.Printf("VVVVVVVV=%#v=VVVVVVVVVVV\n", machineStatus)

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

	if len(vms) > 1 {
		return false, fmt.Errorf("Machine.Exist more than one machine found with this name %s", machine.Name)
	}

	return len(vms) == 1, nil
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (*Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	klog.Infof("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)

	machineStatus, err := exoscalev1.MachineSpecFromMachineStatus(machine.Status.ProviderStatus)
	if err != nil {
		return "", fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	if machineStatus.IP == nil {
		return "", errors.New("could not get IP")
	}

	return machineStatus.IP.String(), nil
}

// GetKubeConfig gets a kubeconfig from the master.
func (*Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	klog.Infof("Getting IP of machine %v for cluster %v.", master.Name, cluster.Name)

	machineStatus, err := exoscalev1.MachineSpecFromMachineStatus(master.Status.ProviderStatus)
	if err != nil {
		return "", fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	sshclient, err := exossh.NewSSHClient(machineStatus.IP.String(), machineStatus.User, machineStatus.SSHPrivateKey)
	if err != nil {
		return "", fmt.Errorf("unable to initialize SSH client: %s", err)
	}

	var stdout, stderr io.Writer

	if err := sshclient.RunCommand("sudo cat /etc/kubernetes/admin.conf", stdout, stderr); err != nil {
		return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() failed to run ssh cmd: %v", err)
	}

	kubeconfig := fmt.Sprint(stdout)
	println("KKKKKKK:", kubeconfig, ":KKKKKKK")

	return kubeconfig, nil
}
