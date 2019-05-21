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

	masterSecurityGroup := clusterSpec.MasterSecurityGroup
	if masterSecurityGroup == "" {
		masterSecurityGroup = cluster.Name + "-master"
	}
	nodeSecurityGroup := clusterSpec.NodeSecurityGroup
	if nodeSecurityGroup == "" {
		nodeSecurityGroup = cluster.Name + "-node"
	}

	//XXX can be possible to authorize sg in each other
	masterRules := buildMasterFirewallRules(masterSecurityGroup, nodeSecurityGroup)
	nodeRules := buildNodeFirewallRules(nodeSecurityGroup, masterSecurityGroup)

	masterSGID := clusterStatus.MasterSecurityGroupID
	nodeSGID := clusterStatus.NodeSecurityGroupID
	if masterSGID == nil {
		sg, err := getORCreateSecurityGroup(masterSecurityGroup)
		if err != nil {
			return err
		}

		masterSGID = sg.ID

	} else {
		klog.Infof("using existing security group id %s", clusterStatus.MasterSecurityGroupID)
	}
	if nodeSGID == nil {
		sg, err := getORCreateSecurityGroup(nodeSecurityGroup)
		if err != nil {
			return err
		}

		nodeSGID = sg.ID
	} else {
		klog.Infof("using existing security group id %s", clusterStatus.NodeSecurityGroupID)
	}

	if err := checkSecurityGroup(masterSGID, masterRules); err != nil {
		return err
	}
	if err := checkSecurityGroup(nodeSGID, nodeRules); err != nil {
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
	klog.Infof("reconcile cluster %q success", cluster.Name)

	return nil
}

func getORCreateSecurityGroup(sgName string) (*egoscale.SecurityGroup, error) {
	exoClient, err := exoclient.Client()
	if err != nil {
		return nil, err
	}

	resp, err := exoClient.Get(egoscale.SecurityGroup{Name: sgName})
	if err != nil {
		if e, ok := err.(*egoscale.ErrorResponse); ok {
			if e.ErrorCode != egoscale.ParamError {
				return nil, err
			}
		}
	}

	if err == nil {
		return resp.(*egoscale.SecurityGroup), nil
	}

	resp, err = exoClient.Request(egoscale.CreateSecurityGroup{Name: sgName})
	if err != nil {
		return nil, fmt.Errorf("error creating network security group: %v", err)
	}

	return resp.(*egoscale.SecurityGroup), nil
}

func checkSecurityGroup(sgID *egoscale.UUID, rules []egoscale.AuthorizeSecurityGroupIngress) error {
	exoClient, err := exoclient.Client()
	if err != nil {
		return err
	}

	resp, err := exoClient.Get(egoscale.SecurityGroup{ID: sgID})
	if err != nil {
		return err
	}
	sg := resp.(*egoscale.SecurityGroup)

	if len(sg.IngressRule) > 0 {
		return nil
	}

	for _, rule := range rules {
		rule.SecurityGroupID = sg.ID
		_, err = exoClient.Request(rule)
		if err != nil {
			return fmt.Errorf("error creating or updating security group rule: %v", err)
		}
	}

	return nil
}

func buildMasterFirewallRules(self, nodeSG string) []egoscale.AuthorizeSecurityGroupIngress {
	return []egoscale.AuthorizeSecurityGroupIngress{
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 80,
			EndPort:   80,
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
			Description: "HTTP",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 22,
			EndPort:   22,
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
			Description: "SSH",
		},
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
				egoscale.UserSecurityGroup{Group: self},
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
				egoscale.UserSecurityGroup{Group: self},
			},
			Description: "Kubelet API, kube-scheduler, kube-controller-manager",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 179,
			EndPort:   179,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: nodeSG},
			},
			Description: "Calico BGP",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 6666,
			EndPort:   6666,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: nodeSG},
			},
			Description: "Calico etcd",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol: "ipip",
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: nodeSG},
			},
			Description: "Calico IPIP",
		},
	}
}

func buildNodeFirewallRules(self, masterSG string) []egoscale.AuthorizeSecurityGroupIngress {
	return []egoscale.AuthorizeSecurityGroupIngress{
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 80,
			EndPort:   80,
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
			Description: "HTTP",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 22,
			EndPort:   22,
			CIDRList: []egoscale.CIDR{
				*egoscale.MustParseCIDR("0.0.0.0/0"),
				*egoscale.MustParseCIDR("::/0"),
			},
			Description: "SSH",
		},
		//XXX if you move Control plane from master sg in an other security group please update this rule
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 10250,
			EndPort:   10250,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: masterSG},
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
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol:  "TCP",
			StartPort: 179,
			EndPort:   179,
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: masterSG},
			},
			Description: "Calico BGP",
		},
		egoscale.AuthorizeSecurityGroupIngress{
			Protocol: "ipip",
			UserSecurityGroupList: []egoscale.UserSecurityGroup{
				egoscale.UserSecurityGroup{Group: self},
				egoscale.UserSecurityGroup{Group: masterSG},
			},
			Description: "Calico IPIP",
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

	allRules := make([]egoscale.IngressRule, 0)

	sgs, err := exoClient.List(egoscale.SecurityGroup{ID: clusterStatus.MasterSecurityGroupID})
	if err != nil {
		return fmt.Errorf("failed to get securityGroup: %v", err)
	}
	if len(sgs) == 1 {
		sg := sgs[0].(*egoscale.SecurityGroup)
		allRules = append(allRules, sg.IngressRule...)
	}

	sgs, err = exoClient.List(egoscale.SecurityGroup{ID: clusterStatus.NodeSecurityGroupID})
	if err != nil {
		return fmt.Errorf("failed to get securityGroup: %v", err)
	}
	if len(sgs) == 1 {
		sg := sgs[0].(*egoscale.SecurityGroup)
		allRules = append(allRules, sg.IngressRule...)
	}

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
func (a *Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	klog.Infof("Getting Kubeconfig of the machine %v for cluster %v.", master.Name, cluster.Name)

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

	res, err := sshclient.QuickCommand("sudo cat /etc/kubernetes/admin.conf")
	if err != nil {
		return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() failed to run ssh cmd: %v", err)
	}

	return res, nil
}
