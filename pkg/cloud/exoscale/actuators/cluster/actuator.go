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

package cluster

import (
	"errors"
	"fmt"
	"io"
	"time"

	ssh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"

	"github.com/exoscale/egoscale"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exoclient "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/client"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

// Actuator is responsible for performing cluster reconciliation
type Actuator struct {
	clustersGetter client.ClustersGetter
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClustersGetter client.ClustersGetter
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clustersGetter: params.ClustersGetter,
	}, nil
}

// Reconcile reconciles a cluster and is invoked by the Cluster Controller
func (a *Actuator) Reconcile(cluster *clusterv1.Cluster) error {
	klog.Infof("Reconciling cluster %v.", cluster.Name)

	clusterSpec, err := exoscalev1.ClusterSpecFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("error loading cluster provider config: %v", err)
	}

	clusterStatus, err := exoscalev1.ClusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("error loading cluster provider config: %v", err)
	}

	if clusterStatus.SecurityGroupID != nil {
		klog.Infof("using existing security group id %s", clusterStatus.SecurityGroupID)
		return nil
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	sgs, err := exoClient.List(&egoscale.SecurityGroup{Name: clusterSpec.SecurityGroup})
	if err != nil {
		return fmt.Errorf("error getting network security group: %v", err)
	}

	var sgID *egoscale.UUID
	if len(sgs) == 0 {
		req := egoscale.CreateSecurityGroup{
			Name: clusterSpec.SecurityGroup,
		}

		klog.Infof("creating security group %q", clusterSpec.SecurityGroup)

		resp, err := exoClient.Request(req)
		if err != nil {
			return fmt.Errorf("error creating or updating network security group: %v", err)
		}
		sgID = resp.(*egoscale.SecurityGroup).ID

		_, err = exoClient.Request(egoscale.AuthorizeSecurityGroupIngress{
			SecurityGroupID: sgID,
			CIDRList:        []egoscale.CIDR{*egoscale.MustParseCIDR("0.0.0.0/0")},
			Protocol:        "ALL",
		})
		if err != nil {
			return fmt.Errorf("error creating or updating security group rule: %v", err)
		}

	} else {
		sgID = sgs[0].(*egoscale.SecurityGroup).ID
	}

	// Put the data into the "Status"
	clusterStatus = &exoscalev1.ExoscaleClusterProviderStatus{
		metav1.TypeMeta{
			Kind:       "ExoscaleClusterProviderStatus",
			APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
		},
		metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{time.Now()},
		},
		sgID,
	}

	if err := a.updateResources(clusterStatus, cluster); err != nil {
		return fmt.Errorf("error updating cluster resources: %v", err)
	}

	return nil
}

func (a *Actuator) updateResources(clusterStatus *exoscalev1.ExoscaleClusterProviderStatus, cluster *clusterv1.Cluster) error {
	rawStatus, err := json.Marshal(clusterStatus)
	if err != nil {
		return err
	}

	cluster.Status.ProviderStatus = &runtime.RawExtension{
		Raw: rawStatus,
	}

	clusterClient := a.clustersGetter.Clusters(cluster.Namespace)

	if _, err := clusterClient.UpdateStatus(cluster); err != nil {
		return err
	}

	return nil
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	klog.Infof("Deleting cluster %v.", cluster.Name)

	clusterStatus, err := exoscalev1.ClusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("error loading cluster provider config: %v", err)
	}

	if clusterStatus.SecurityGroupID == nil {
		klog.Infof("no security group id to be deleted, skip")
		return nil
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	sg, err := exoClient.Get(egoscale.SecurityGroup{ID: clusterStatus.SecurityGroupID})
	if err != nil {
		return fmt.Errorf("failed to get securityGroup: %v", err)
	}
	securityGroup := sg.(*egoscale.SecurityGroup)

	for _, r := range securityGroup.IngressRule {
		if err := exoClient.BooleanRequest(egoscale.RevokeSecurityGroupIngress{ID: r.RuleID}); err != nil {
			return fmt.Errorf("failed to revoke securityGroup ingress rule: %v", err)
		}
	}

	return exoClient.BooleanRequest(egoscale.DeleteSecurityGroup{
		ID: clusterStatus.SecurityGroupID,
	})
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

	sshclient, err := ssh.NewSSHClient(machineStatus.IP.String(), machineStatus.User, machineStatus.SSHPrivateKey)
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
