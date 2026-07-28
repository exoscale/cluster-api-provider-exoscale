# Kubernetes Cluster API Provider Exoscale

## What is the Cluster API Provider Exoscale

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to
cluster creation, configuration and management.

Exoscale CAPI is an infrastructure provider that provisions and manages Exoscale
resources for self-managed Kubernetes clusters. It currently reconciles
`ExoscaleCluster` and `ExoscaleMachine` resources into the required cloud
infrastructure.

## Run locally

Prerequisites: Docker, `kind`, `kubectl`, the Go version declared in `go.mod`,
and Exoscale API credentials.

### Deploy Cluster API components
```bash
$> make clusterctl
$> kind create cluster --name capi-test
$> ./bin/clusterctl init --infrastructure - # installs CAPI core and kubeadm providers
```

### Run CAPI
```bash
$> make generate manifests install
$> make run
```

Keep the manager running and use another terminal for the remaining commands.

### Deploy a simple cluster
```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/cluster/
```

#### Wait for the workload cluster
```bash
$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=10m
$> ./bin/clusterctl get kubeconfig my-cluster > /tmp/my-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
$> kubectl wait cluster/my-cluster --for=condition=RemoteConnectionProbe --timeout=5m
$> kubectl wait machine/my-control-plane --for=condition=Ready --for=condition=Available --timeout=5m
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig get nodes

$> kubectl get exoscalecluster,machine,exoscalemachine,kubeadmconfig
```

#### Run a workload smoke test
```bash
kubectl --kubeconfig=/tmp/my-cluster.kubeconfig run smoke --image=busybox:1.36 --restart=Never --rm --attach --command -- sh -c 'echo "Hello from $(hostname)"'
```

This creates a Pod, prints its hostname, and deletes it after completion.

### Delete simple cluster
```bash
$> kubectl delete cluster/my-cluster # also deletes its Machine and Exoscale resources
```

The shared `exoscale` credential Secret is not owned by the Cluster and remains
after Cluster deletion.

## End-to-End testing
```bash
$> export EXOSCALE_API_KEY=<api-key>       # Optional if exocli is not configured
$> export EXOSCALE_API_SECRET=<api-secret> # Optional if exocli is not configured

$> make chainsaw-test-e2e
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

[cluster_api]: https://github.com/kubernetes-sigs/cluster-api
