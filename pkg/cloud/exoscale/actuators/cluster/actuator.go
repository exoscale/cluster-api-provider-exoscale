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

package cluster

import (
	"errors"
	"fmt"
	"log"

	"k8s.io/klog"
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
	log.Printf("Reconciling cluster %v.", cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	log.Printf("Deleting cluster %v.", cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// // Deprecated interface for Provider specific logic. Please do not extend or add. This interface should be removed
// // once issues/158 and issues/160 below are fixed.
// type ProviderDeployer interface {
// 	// TODO: This requirement can be removed once after: https://github.com/kubernetes-sigs/cluster-api/issues/158
// 	GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error)
// 	// TODO: This requirement can be removed after: https://github.com/kubernetes-sigs/cluster-api/issues/160
// 	GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error)
// }

//GetIP return IP of deployed instance in actuator
func (*Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if ip, ok := machine.ObjectMeta.Annotations[ExoscaleIPAnnotationKey]; ok {
			klog.Infof("Returning IP from machine annotation %s", ip)
			return ip, nil
		}
	}

	return "", errors.New("could not get IP")
}

//GetKubeConfig return kube config of deployed k8s cluster
func (*Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	return "", fmt.Errorf("Provisionner exoscale GetKubeConfig() not yet implemented")
}
