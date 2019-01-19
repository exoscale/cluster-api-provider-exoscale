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

	"github.com/exoscale/egoscale"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"
	exoscalev1 "sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis/exoscale/v1alpha1"
	exoclient "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/client"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
	ExoscaleIPAnnotationKey = "exoscale-ip"
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

	println("START: cluster.actuator.create")

	clusterSpec, err := clusterSpecFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("error loading cluster provider config: %v", err)
	}

	clusterStatus, err := clusterStatusFromProviderStatus(cluster.Status.ProviderStatus)
	if err != nil {
		return fmt.Errorf("error loading cluster provider config: %v", err)
	}

	if clusterStatus.SecurityGroupID != "" {
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

	var sg *egoscale.SecurityGroup
	if len(sgs) == 0 {
		req := egoscale.CreateSecurityGroup{
			Name: clusterSpec.SecurityGroup,
		}

		resp, err := exoClient.Request(req)
		if err != nil {
			return fmt.Errorf("error creating or updating network security group: %v", err)
		}

		sg = resp.(*egoscale.SecurityGroup)
	} else {
		sg = sgs[0].(*egoscale.SecurityGroup)
	}

	// Put the data into the "Status"
	clusterStatus.SecurityGroupID = sg.ID.String()

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

	println("END: cluster.actuator.create")

	return nil
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	klog.Infof("Deleting cluster %v.", cluster.Name)

	klog.Error("deleting a cluster is not yet implemented")
	return nil
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

func clusterStatusFromProviderStatus(providerStatus *runtime.RawExtension) (*exoscalev1.ExoscaleClusterProviderStatus, error) {
	config := new(exoscalev1.ExoscaleClusterProviderStatus)
	if providerStatus != nil {
		if err := yaml.Unmarshal(providerStatus.Raw, config); err != nil {
			return nil, err
		}
	}
	return config, nil
}
