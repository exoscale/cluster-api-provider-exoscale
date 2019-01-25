/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"

	"k8s.io/klog"
	"sigs.k8s.io/cluster-api-provider-exoscale/pkg/apis"
	"sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/cluster"
	"sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/machine"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
	capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

// initLogs is a temporary hack to enable proper logging until upstream dependencies
// are migrated to fully utilize klog instead of glog.
func initLogs() {
	flag.Set("logtostderr", "true") // nolint: errcheck
	flags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(flags)
	flags.Set("alsologtostderr", "true") // nolint: errcheck
	flags.Set("v", "3")                  // nolint: errcheck
	flag.Parse()
}

func main() {
	initLogs()
	cfg := config.GetConfigOrDie()
	if cfg == nil {
		panic(fmt.Errorf("GetConfigOrDie didn't die"))
	}

	// Setup a Manager
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		klog.Fatalf("unable to set up overall controller manager: %v", err)
	}

	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("unable to create client from configuration: %v", err)
	}

	clusterActuator, err := cluster.NewActuator(cluster.ActuatorParams{
		ClustersGetter: cs.ClusterV1alpha1(),
	})
	if err != nil {
		panic(err)
	}

	machineActuator, err := machine.NewActuator(machine.ActuatorParams{
		MachinesGetter: cs.ClusterV1alpha1(),
	})
	if err != nil {
		klog.Fatal(err)
	}

	// Register our cluster deployer (the interface is in clusterctl and we define the Deployer interface on the actuator)
	common.RegisterClusterProvisioner("exoscale", clusterActuator)

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	if err := capimachine.AddWithActuator(mgr, machineActuator); err != nil {
		klog.Fatal(err)
	}

	if err := capicluster.AddWithActuator(mgr, clusterActuator); err != nil {
		klog.Fatal(err)
	}

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		klog.Fatalf("unable to run manager: %v", err)
	}
}
