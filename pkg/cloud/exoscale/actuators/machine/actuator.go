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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	klog.V(1).Infof("Creating machine %v for cluster %v.", machine.Name, cluster.Name)

	clusterStatus, err := exoscalev1.ClusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal cluster.Status field: %v", err)
	}

	machineConfig, err := exoscalev1.MachineSpecFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	securityGroup := clusterStatus.MasterSecurityGroupID
	if !isMasterNode(machine) {
		securityGroup = clusterStatus.NodeSecurityGroupID
	}
	if securityGroup == nil {
		return fmt.Errorf("empty masterSecurityGroupID or nodeSecurityGroupID field. %#v", clusterStatus)
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

	//sshKeyName := machine.Name + "_id_rsa"
	sshKeyName := ""
	if machineConfig.SSHKey != "" {
		sshKeyName = machineConfig.SSHKey
	}

	/*
		keyPairs, err := exossh.CreateSSHKey(ctx, exoClient, sshKeyName)
		if err != nil {
			r := err.(*egoscale.ErrorResponse)
			if r.ErrorCode != egoscale.ParamError && r.CSErrorCode != egoscale.InvalidParameterValueException {
				return err
			}
			return fmt.Errorf("an SSH key with that name %q already exists, please choose a different name", sshKeyName)
		}
	*/
	println("MACHINESET.LABEL:", machine.ObjectMeta.Labels["set"])

	req := egoscale.DeployVirtualMachine{
		Name:              machine.Name,
		ZoneID:            zone.ID,
		TemplateID:        template.ID,
		RootDiskSize:      machineConfig.Disk,
		SecurityGroupIDs:  []egoscale.UUID{*securityGroup},
		ServiceOfferingID: serviceOffering.ID,
		KeyPair:           sshKeyName,
	}

	resp, err := exoClient.RequestWithContext(ctx, req)
	if err != nil {
		//cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("exoscale failed to DeployVirtualMachine %v", err)
	}
	vm := resp.(*egoscale.VirtualMachine)

	klog.V(4).Infof("Deployed instance: %q, IP: %s, password: %q", vm.Name, vm.IP().String(), vm.Password)

	klog.V(1).Infof("Provisioning (can take up to several minutes):")

	machineSet := machine.ObjectMeta.Labels["set"]
	switch machineSet {
	case "master":
		err = a.provisionMaster(machine, vm, username)
	case "node":
		err = a.provisionNode(cluster, machine, vm, username)
	default:
		err = fmt.Errorf(`invalide machine set: %q expected "master" or "node" only`, machineSet)
	}

	if err != nil {
		//cleanSSHKey(exoClient, keyPairs.Name)
		return err
	}

	klog.V(1).Infof("Machine %q provisioning success!", machine.Name)

	// XXX annotations should be replaced by the proper NodeRef
	// https://github.com/kubernetes-sigs/cluster-api/blob/3b5183805f4dbf859d39a2600b268192a8191950/cmd/clusterctl/clusterdeployer/clusterclient/clusterclient.go#L579-L581
	annotations := machine.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[exoscalev1.ExoscaleIPAnnotationKey] = vm.IP().String()
	annotations[exoscalev1.ExoscaleUsernameAnnotationKey] = username
	annotations[exoscalev1.ExoscalePasswordAnnotationKey] = vm.Password
	machine.SetAnnotations(annotations)

	machineClient := a.machinesGetter.Machines(machine.Namespace)
	newMachine, err := machineClient.Update(machine)
	if err != nil {
		return err
	}

	machineStatus := &exoscalev1.ExoscaleMachineProviderStatus{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExoscaleMachineProviderStatus",
			APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		ID:         vm.ID,
		IP:         *vm.IP(),
		TemplateID: vm.TemplateID,
		User:       username,
		Password:   vm.Password,
		ZoneID:     vm.ZoneID,
	}

	if err := a.updateResources(newMachine, machineStatus); err != nil {
		//cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("failed to update machine resources: %s", err)
	}

	return nil
}

func (a *Actuator) updateResources(machine *clusterv1.Machine, machineStatus *exoscalev1.ExoscaleMachineProviderStatus) error {
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

func isMasterNode(machine *clusterv1.Machine) bool {
	return machine.ObjectMeta.Labels["set"] == "master"
}

func cleanSSHKey(exoClient *egoscale.Client, sshKeyName string) {
	_ = exoClient.Delete(egoscale.SSHKeyPair{Name: sshKeyName})
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if cluster == nil {
		klog.Warningf("cluster %q as been removed already", machine.Name)
		klog.V(1).Infof("deleting machine %q.", machine.Name)
	} else {
		klog.V(1).Infof("deleting machine %q from %q.", machine.Name, cluster.Name)
	}

	machineStatus, err := exoscalev1.MachineStatusFromProviderStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("cannot unmarshal machine.Spec field: %v", err)
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	/*
		if err := exoClient.Delete(egoscale.SSHKeyPair{Name: machineStatus.SSHKeyName}); err != nil {
			return fmt.Errorf("cannot delete machine SSH KEY: %v", err)
		}
	*/

	err = exoClient.Delete(egoscale.VirtualMachine{
		ID: machineStatus.ID,
	})
	// It was already deleted externally
	if e, ok := err.(*egoscale.ErrorResponse); ok {
		if e.ErrorCode == egoscale.ParamError {
			return nil
		}
	}

	return err
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	klog.V(1).Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	klog.Warningf("Updating a machine is not yet implemented")

	machineStatus, err := exoscalev1.MachineStatusFromProviderStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Status.ProviderStatus field: %v", err)
	}

	if machineStatus == nil {
		// redoing machine status...

		exoClient, err := exoclient.Client()
		if err != nil {
			return err
		}

		resp, err := exoClient.GetWithContext(ctx, &egoscale.VirtualMachine{Name: machine.Name})
		if err != nil {
			return err
		}

		vm := resp.(*egoscale.VirtualMachine)

		// dirty trick
		annotations := machine.GetAnnotations()
		if annotations == nil {
			return errors.New("could not get the annotations")
		}

		password, ok := annotations[exoscalev1.ExoscalePasswordAnnotationKey]
		if !ok {
			return errors.New("could not get password from the annotations")
		}
		vm.Password = password

		username, ok := annotations[exoscalev1.ExoscaleUsernameAnnotationKey]
		if !ok {
			return errors.New("could not get password from the annotations")
		}

		machineStatus := &exoscalev1.ExoscaleMachineProviderStatus{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExoscaleMachineProviderStatus",
				APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			ID:         vm.ID,
			IP:         *vm.IP(),
			TemplateID: vm.TemplateID,
			User:       username,
			Password:   vm.Password,
			ZoneID:     vm.ZoneID,
		}

		if err := a.updateResources(machine, machineStatus); err != nil {
			return fmt.Errorf("failed to update machine resources: %s", err)
		}
	}

	return nil
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	klog.V(1).Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)

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
	klog.V(1).Infof("Getting IP of the machine %v for cluster %v.", machine.Name, cluster.Name)

	annotations := machine.GetAnnotations()
	if annotations == nil {
		return "", errors.New("could not get the annotations")
	}

	ip, ok := annotations[exoscalev1.ExoscaleIPAnnotationKey]
	if !ok {
		return "", errors.New("could not get IP from the annotations")
	}
	return ip, nil
}

// GetKubeConfig gets a kubeconfig from the master.
func (*Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	klog.V(1).Infof("Getting Kubeconfig of the machine %v for cluster %v.", master.Name, cluster.Name)

	masterIP, ok := master.Annotations[exoscalev1.ExoscaleIPAnnotationKey]
	if !ok {
		return "", fmt.Errorf("failed to get IP in master machine spec: %q", master.Name)
	}
	user, ok := master.Annotations[exoscalev1.ExoscaleUsernameAnnotationKey]
	if !ok {
		return "", fmt.Errorf("failed to get user in master machine spec: %q", master.Name)
	}
	password, ok := master.Annotations[exoscalev1.ExoscalePasswordAnnotationKey]
	if !ok {
		return "", fmt.Errorf("failed to get password in master machine spec: %q", master.Name)
	}

	sshclient := exossh.NewSSHClient(masterIP, user, password)

	var buf bytes.Buffer
	if err := sshclient.RunCommand("sudo cat /etc/kubernetes/admin.conf", &buf, nil); err != nil {
		return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() failed to run ssh cmd: %v", err)
	}

	return buf.String(), nil
}

func (a *Actuator) getControlPlaneMachines(machineList *clusterv1.MachineList) []*clusterv1.Machine {
	var cpm []*clusterv1.Machine
	for _, m := range machineList.Items {
		if m.Spec.Versions.ControlPlane != "" {
			klog.V(0).Infof("controlplane %q", m.Spec.Versions.ControlPlane)
			cpm = append(cpm, m.DeepCopy())
		}
	}
	return cpm
}
