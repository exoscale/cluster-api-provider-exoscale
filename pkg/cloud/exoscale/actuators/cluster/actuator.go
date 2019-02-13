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
	"time"

	"github.com/exoscale/egoscale"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exossh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"
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

	if clusterStatus.MasterSecurityGroupID != nil {
		klog.Infof("using existing security group id %s", clusterStatus.MasterSecurityGroupID)
		return nil
	}
	if clusterStatus.NodeSecurityGroupID != nil {
		klog.Infof("using existing security group id %s", clusterStatus.NodeSecurityGroupID)
		return nil
	}

	//XXX can be possible to authorize sg in each other
	masterRules := createMasterFirewallRules(clusterSpec.MasterSecurityGroup)
	nodeRules := createNodeFirewallRules(clusterSpec.NodeSecurityGroup, clusterSpec.MasterSecurityGroup)

	masterSGID, err := checkSecurityGroup(clusterSpec.MasterSecurityGroup, masterRules)
	if err != nil {
		//XXX if fail clean sg and delete it
		return err
	}
	nodeSGID, err := checkSecurityGroup(clusterSpec.NodeSecurityGroup, nodeRules)
	if err != nil {
		//XXX if fail clean sg and delete it
		return err
	}

	// Put the data into the "Status"
	clusterStatus = &exoscalev1.ExoscaleClusterProviderStatus{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExoscaleClusterProviderStatus",
			APIVersion: "exoscale.cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		MasterSecurityGroupID: masterSGID,
		NodeSecurityGroupID:   nodeSGID,
	}

	if err := a.updateResources(clusterStatus, cluster); err != nil {
		return fmt.Errorf("error updating cluster resources: %v", err)
	}

	return nil
}

func checkSecurityGroup(sgName string, rules []egoscale.AuthorizeSecurityGroupIngress) (*egoscale.UUID, error) {
	exoClient, err := exoclient.Client()
	if err != nil {
		return nil, err
	}

	sgs, err := exoClient.List(&egoscale.SecurityGroup{Name: sgName})
	if err != nil {
		return nil, fmt.Errorf("error getting network security group: %v", err)
	}

	var sgID *egoscale.UUID
	if len(sgs) == 0 {
		req := egoscale.CreateSecurityGroup{
			Name: sgName,
		}

		klog.Infof("creating security group %q", sgName)

		resp, err := exoClient.Request(req)
		if err != nil {
			return nil, fmt.Errorf("error creating or updating network security group: %v", err)
		}
		sgID = resp.(*egoscale.SecurityGroup).ID

		for _, rule := range rules {
			rule.SecurityGroupID = sgID
			_, err = exoClient.Request(rule)
			if err != nil {

				return nil, fmt.Errorf("error creating or updating security group rule: %v", err)
			}
		}
	} else {
		sgID = sgs[0].(*egoscale.SecurityGroup).ID
	}
	return sgID, nil

}

func createMasterFirewallRules(self string) []egoscale.AuthorizeSecurityGroupIngress {
	return []egoscale.AuthorizeSecurityGroupIngress{
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 6443,
			EndPort:   6443,
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
			Description: "Kubernetes API server",
		},
		///XXX TODO if etcd is in an other security group careful you must update this rule
		//     or make it dynamic if you leave the choice to the provider spec
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 2379,
			EndPort:   2380,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{self},
			},
			Description: "etcd server client API",
		},
		//XXX if you move Control plane from master sg in an other security group
		//    please create a new one to accept ingress from Control plane
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 10250,
			EndPort:   10252,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{self},
			},
			Description: "Kubelet API, kube-scheduler, kube-controller-manager",
		},
	}
}

func createNodeFirewallRules(self, ingressSG string) []egoscale.AuthorizeSecurityGroupIngress {
	return []egoscale.AuthorizeSecurityGroupIngress{
		//XXX if you move Control plane from master sg in an other security group please update this rule
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 10250,
			EndPort:   10250,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{self},
				egoscale.UserSecurityGroup{ingressSG},
			},
			Description: "Kubelet API",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:    "TCP",
			StartPort:   30000,
			EndPort:     32767,
			Description: "NodePort Services",
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
		},
	}
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

	if clusterStatus.MasterSecurityGroupID == nil && clusterStatus.NodeSecurityGroupID == nil {
		klog.Infof("no security group id to be deleted, skip")
		return nil
	}

	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	sg, err := exoClient.Get(egoscale.SecurityGroup{ID: clusterStatus.MasterSecurityGroupID})
	if err != nil {
		return fmt.Errorf("failed to get securityGroup: %v", err)
	}
	masterSecurityGroup := sg.(*egoscale.SecurityGroup)

	sg, err = exoClient.Get(egoscale.SecurityGroup{ID: clusterStatus.NodeSecurityGroupID})
	if err != nil {
		return fmt.Errorf("failed to get securityGroup: %v", err)
	}
	nodeSecurityGroup := sg.(*egoscale.SecurityGroup)

	allRules := append(masterSecurityGroup.IngressRule, nodeSecurityGroup.IngressRule...)

	for _, r := range allRules {
		if err := exoClient.BooleanRequest(egoscale.RevokeSecurityGroupIngress{ID: r.RuleID}); err != nil {
			return fmt.Errorf("failed to revoke securityGroup ingress rule: %v", err)
		}
	}

	err = exoClient.BooleanRequest(egoscale.DeleteSecurityGroup{
		ID: clusterStatus.MasterSecurityGroupID,
	})

	err = exoClient.BooleanRequest(egoscale.DeleteSecurityGroup{
		ID: clusterStatus.NodeSecurityGroupID,
	})

	return err
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (*Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	klog.Infof("Getting IP of the machine %v for cluster %v.", machine.Name, cluster.Name)

	annotations := machine.GetAnnotations()
	if annotations == nil {
		return "", errors.New("could not get IP")
	}

	return annotations[exoscalev1.ExoscaleIPAnnotationKey], nil
}

// GetKubeConfig gets a kubeconfig from the master.
func (*Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	klog.Infof("Getting Kubeconfig of the machine %v for cluster %v.", master.Name, cluster.Name)

	machineStatus, err := exoscalev1.MachineStatusFromProviderStatus(master.Status.ProviderStatus)
	if err != nil {
		return "", fmt.Errorf("Cannot unmarshal machine.Spec field: %v", err)
	}

	sshclient, err := exossh.NewSSHClient(machineStatus.IP.String(), machineStatus.User, machineStatus.SSHPrivateKey)
	if err != nil {
		return "", fmt.Errorf("unable to initialize SSH client: %s", err)
	}

	res, err := sshclient.QuickCommand("sudo cat /etc/kubernetes/admin.conf")
	if err != nil {
		return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() failed to run ssh cmd: %v", err)
	}

	return res, nil
}
