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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exoscale/egoscale"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exossh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"
	exoclient "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/client"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	capierror "sigs.k8s.io/cluster-api/pkg/controller/error"
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
	klog.V(1).Infof("Creating machine %q, for cluster %#v.", machine.Name, cluster)
	if cluster == nil {
		return fmt.Errorf("missing cluster for machine %s/%s", machine.Namespace, machine.Name)
	}

	clusterStatus, err := exoscalev1.ClusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal cluster.Status field: %v", err)
	}

	machineProviderSpec, err := exoscalev1.MachineSpecFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	securityGroup := clusterStatus.MasterSecurityGroupID
	if !isMasterNode(machine) {
		securityGroup = clusterStatus.NodeSecurityGroupID
	}
	if securityGroup == nil {
		return &capierror.RequeueAfterError{RequeueAfter: time.Second}
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	z, err := exoClient.GetWithContext(ctx, &egoscale.Zone{Name: machineProviderSpec.Zone})
	if err != nil {
		return fmt.Errorf("problem fetching the zone %q. %s", machineProviderSpec.Zone, err)
	}
	zone := z.(*egoscale.Zone)

	t, err := exoClient.GetWithContext(
		ctx,
		&egoscale.Template{
			Name:       machineProviderSpec.Template,
			ZoneID:     zone.ID,
			IsFeatured: true,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching the template %q. %s", machineProviderSpec.Template, err)
	}
	template := t.(*egoscale.Template)
	username, ok := template.Details["username"]
	if !ok {
		return fmt.Errorf("problem fetching username for template %q", template.Name)
	}

	so, err := exoClient.GetWithContext(
		ctx,
		&egoscale.ServiceOffering{
			Name: machineProviderSpec.Type,
		},
	)
	if err != nil {
		return fmt.Errorf("problem fetching service-offering %q. %s", machineProviderSpec.Type, err)
	}
	serviceOffering := so.(*egoscale.ServiceOffering)

	//sshKeyName := machine.Name + "_id_rsa"
	sshKeyName := ""
	if machineProviderSpec.SSHKey != "" {
		sshKeyName = machineProviderSpec.SSHKey
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

	klog.V(4).Infof("MACHINESET.LABEL: %q", machine.ObjectMeta.Labels["set"])

	var userData string
	if machineProviderSpec.Cloudinit != "" {
		userData = base64.StdEncoding.EncodeToString([]byte(machineProviderSpec.Cloudinit))
	}

	req := egoscale.DeployVirtualMachine{
		Name:              machine.Name,
		ZoneID:            zone.ID,
		TemplateID:        template.ID,
		RootDiskSize:      machineProviderSpec.Disk,
		SecurityGroupIDs:  []egoscale.UUID{*securityGroup},
		ServiceOfferingID: serviceOffering.ID,
		KeyPair:           sshKeyName,
		UserData:          userData,
	}

	result, err := exoClient.SyncRequestWithContext(ctx, req)
	if err != nil {
		return err
	}

	jobResult, ok := result.(*egoscale.AsyncJobResult)
	if !ok {
		return fmt.Errorf("wrong type, AsyncJobResult was expected instead of %T", result)
	}

	vm := new(egoscale.VirtualMachine)
	// Successful response
	if jobResult.JobID == nil || jobResult.JobStatus != egoscale.Pending {
		if errR := jobResult.Result(vm); errR != nil {
			return errR
		}
		klog.V(4).Infof("Deployed instance: %q, IP: %s, password: %q", vm.Name, vm.IP().String(), vm.Password)
	}

	if vm.ID != nil {
		return a.AddVirtualMachineTOMachineStatus(vm, machine, username)
	}

	machineStatus := &exoscalev1.ExoscaleMachineProviderStatus{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExoscaleMachineProviderStatus",
			APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		AsyncJobResult:    jobResult,
		User:              username,
		ZoneID:            zone.ID,
		TemplateID:        template.ID,
		ServiceOfferingID: serviceOffering.ID,
	}
	machinePhase := exoscalev1.MachinePhaseBooting
	machine.Status.Phase = &machinePhase
	if err := a.updateResources(machine, machineStatus); err != nil {
		//cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("failed to update machine resources: %s", err)
	}

	return nil
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	klog.V(1).Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)

	machineStatus, err := exoscalev1.MachineStatusFromProviderStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Status.ProviderStatus field: %v", err)
	}

	annotations := machineGetAnnotations(machine)
	if machine.Status.Phase == nil && annotations[exoscalev1.ExoscaleIPAnnotationKey] == "" {
		klog.Warningf("Error machine %q not created", machine.Name)
		return a.Create(ctx, cluster, machine)
	}

	if machine.Status.Phase == nil {
		machinePhase := exoscalev1.MachinePhaseReady
		machine.Status.Phase = &machinePhase

		if err := a.updateResources(machine, machineStatus); err != nil {
			return fmt.Errorf("failed to update machine resources: %v", err)
		}

		return nil
	}

	// Not possible but try
	if *machine.Status.Phase == exoscalev1.MachinePhaseDeleting {
		return nil
	}

	if *machine.Status.Phase == exoscalev1.MachinePhaseFailure {
		machinePhase := exoscalev1.MachinePhasePending
		machine.Status.Phase = &machinePhase

		if err := a.updateResources(machine, machineStatus); err != nil {
			return fmt.Errorf("failed to update machine resources: %v", err)
		}
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	vm := new(egoscale.VirtualMachine)
	if *machine.Status.Phase == exoscalev1.MachinePhaseBooting {
		req := &egoscale.QueryAsyncJobResult{JobID: machineStatus.AsyncJobResult.JobID}
		resp, err := exoClient.SyncRequestWithContext(ctx, req)
		if err != nil {
			return err
		}

		result, ok := resp.(*egoscale.AsyncJobResult)
		if !ok {
			return fmt.Errorf("wrong type. AsyncJobResult expected, got %T", resp)
		}

		if result.JobStatus == egoscale.Success {
			if errR := result.Result(vm); errR != nil {
				return errR
			}
			klog.V(4).Infof("Deployed instance: %q, IP: %s, password: %q", vm.Name, vm.IP().String(), vm.Password)

			err := a.AddVirtualMachineTOMachineStatus(vm, machine, machineStatus.User)
			if err != nil {
				return err
			}
		}
		if result.JobStatus == egoscale.Pending {
			return &capierror.RequeueAfterError{RequeueAfter: time.Second}
		}
		if result.JobStatus == egoscale.Failure {
			return a.Create(ctx, cluster, machine)
		}
	}

	machineStatus, err = exoscalev1.MachineStatusFromProviderStatus(machine.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("Cannot unmarshal machine.Status.ProviderStatus field: %v", err)
	}

	if *machine.Status.Phase != exoscalev1.MachinePhasePending &&
		*machine.Status.Phase != exoscalev1.MachinePhaseReady {
		// Should never go in this condition
		//TODO throw error into controller
		klog.Warningf("machine %q is in unknow phase TODO throw error into controller return nil to release controller", machine.Name)
		return nil
	}

	if *machine.Status.Phase == exoscalev1.MachinePhaseReady {
		// Success machine installed
		return nil
	}

	machineSet := strings.ToLower(machine.ObjectMeta.Labels["set"])
	switch machineSet {
	case "master":
		klog.V(1).Infof("Provisioning Master %q (can take up to several minutes):", machine.Name)
		go func() {
			err = a.provisionMaster(machine, machineStatus)
			a.provisioningAsyncResult(err, machine, machineStatus)
		}()
	case "node":
		//XXX work only with 1 master at the moment
		controlPlaneMachine, err := a.getControlPlaneMachine(machine, cluster.Name)
		if err != nil {
			return err
		}
		if controlPlaneMachine.Status.Phase == nil || *controlPlaneMachine.Status.Phase != exoscalev1.MachinePhaseReady {
			return &capierror.RequeueAfterError{RequeueAfter: time.Second}
		}
		klog.V(1).Infof("Provisioning Node %q", machine.Name)
		bootstrapToken, err := a.getNodeJoinToken(cluster, controlPlaneMachine)
		if err != nil {
			return fmt.Errorf("failed to obtain token for node %q to join cluster %q: %v", machine.Name, cluster.Name, err)
		}
		go func() {
			err = a.provisionNode(cluster, machine, machineStatus, bootstrapToken)
			a.provisioningAsyncResult(err, machine, machineStatus)
		}()
	default:
		return fmt.Errorf(`invalid machine set: %q expected "master" or "node" only`, machineSet)
	}

	return nil
}

func (a *Actuator) provisioningAsyncResult(errResult error,
	machine *v1alpha1.Machine,
	mProviderStatus *exoscalev1.ExoscaleMachineProviderStatus) {
	if errResult != nil {
		machinePhase := exoscalev1.MachinePhaseFailure
		machine.Status.Phase = &machinePhase
	} else {
		klog.V(1).Infof("Machine %q provisioning success!", machine.Name)

		machinePhase := exoscalev1.MachinePhaseReady
		machine.Status.Phase = &machinePhase
	}

	if err := a.updateResources(machine, mProviderStatus); err != nil {
		// it should never fail
		klog.Fatalf("failed to update machine resources: %v", err)
	}
}

func (a *Actuator) AddVirtualMachineTOMachineStatus(vm *egoscale.VirtualMachine, machine *v1alpha1.Machine, username string) error {
	// XXX annotations should be replaced by the proper NodeRef
	// https://github.com/kubernetes-sigs/cluster-api/blob/3b5183805f4dbf859d39a2600b268192a8191950/cmd/clusterctl/clusterdeployer/clusterclient/clusterclient.go#L579-L581
	annotations := machineGetAnnotations(machine)
	annotations[exoscalev1.ExoscaleIPAnnotationKey] = vm.IP().String()
	annotations[exoscalev1.ExoscaleUsernameAnnotationKey] = username
	annotations[exoscalev1.ExoscalePasswordAnnotationKey] = vm.Password
	machine.SetAnnotations(annotations)
	machineClient := a.machinesGetter.Machines(machine.Namespace)
	var err error
	machine, err = machineClient.Update(machine)
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

	machinePhase := exoscalev1.MachinePhasePending
	machine.Status.Phase = &machinePhase

	if err := a.updateResources(machine, machineStatus); err != nil {
		//cleanSSHKey(exoClient, keyPairs.Name)
		return fmt.Errorf("failed to update machine resources: %s", err)
	}

	return nil
}

func machineGetAnnotations(machine *v1alpha1.Machine) map[string]string {
	annotations := machine.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	return annotations
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
		return fmt.Errorf("Cannot unmarshal machine.Status.ProviderStatus field: %v", err)
	}

	phase := ""
	if machine.Status.Phase != nil {
		phase = *machine.Status.Phase
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	if phase == exoscalev1.MachinePhaseDeleting {
		req := &egoscale.QueryAsyncJobResult{JobID: machineStatus.AsyncJobResult.JobID}
		resp, err := exoClient.SyncRequestWithContext(ctx, req)
		if err != nil {
			return err
		}

		result, ok := resp.(*egoscale.AsyncJobResult)
		if !ok {
			return fmt.Errorf("wrong type. AsyncJobResult expected, got %T", resp)
		}

		if result.JobStatus == egoscale.Success {
			return nil
		}
		if result.JobStatus == egoscale.Pending {
			return &capierror.RequeueAfterError{RequeueAfter: time.Second * 2}
		}
		if result.JobStatus == egoscale.Failure {
			return a.Delete(ctx, cluster, machine)
		}

	}

	/*
		if err := exoClient.Delete(egoscale.SSHKeyPair{Name: machineStatus.SSHKeyName}); err != nil {
			return fmt.Errorf("cannot delete machine SSH KEY: %v", err)
		}
	*/

	vmID := machineStatus.ID
	if vmID == nil {
		resp, err := exoClient.GetWithContext(ctx, egoscale.VirtualMachine{Name: machine.Name})
		if err != nil {
			return err
		}

		// It was already deleted externally
		if e, ok := err.(*egoscale.ErrorResponse); ok {
			if e.ErrorCode == egoscale.ParamError {
				return nil
			}
		}

		vm := resp.(*egoscale.VirtualMachine)
		vmID = vm.ID
	}

	result, err := exoClient.SyncRequestWithContext(ctx, egoscale.DestroyVirtualMachine{
		ID: vmID,
	})

	jobResult, ok := result.(*egoscale.AsyncJobResult)
	if !ok {
		return fmt.Errorf("wrong type, AsyncJobResult was expected instead of %T", result)
	}

	// Successful response
	if jobResult.JobID == nil || jobResult.JobStatus != egoscale.Pending {
		klog.V(4).Infof("Instance: %q deleted", machine.Name)
		return nil
	}

	machinePhase := exoscalev1.MachinePhaseDeleting
	machine.Status.Phase = &machinePhase
	machineStatus.AsyncJobResult = jobResult
	if err := a.updateResources(machine, machineStatus); err != nil {
		return fmt.Errorf("failed to update machine resources: %s", err)
	}

	return &capierror.RequeueAfterError{RequeueAfter: time.Second * 2}
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	klog.V(1).Infof("Checking if machine %q exists for cluster %#v.", machine.Name, cluster)

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
