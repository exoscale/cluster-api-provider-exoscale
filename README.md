# Kubernetes Cluster API Provider Exoscale

## What is the Cluster API Provider Exoscale (CAPX)

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to
cluster creation, configuration and management.

CAPX is an infrastructure provider that provisions and manages Exoscale
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

### Run CAPX
```bash
$> make generate manifests install
$> ENABLE_WEBHOOKS=false make run
```

Keep the manager running and use another terminal for the remaining commands.

CAPX uses these log verbosity levels:

- V(2) logs controller decisions.
- V(4) logs one safe metadata entry per logical Exoscale SDK call with its
  method, host, path, final status, and total duration including retries. V(4)
  does not include query strings, headers, request or response bodies, or
  credentials.
- V(9) adds one complete request dump and one response dump, including URI,
  query parameters, and headers, only when `--unsafe-exoscale-api-trace` is also
  set. Bodies are excluded.
- V(10) is cumulative and adds request and response bodies to the same V(9)
  dumps.

> **Warning:** V(9) and V(10) can expose API credentials, bootstrap data, and
> other secrets. Enable `--unsafe-exoscale-api-trace` only for short-lived
> debugging in a secured log environment, then disable it and remove the logs.
> The V(4) security guarantee applies only to the V(4) metadata entry, not to
> V(9) or V(10) wire dumps.

For example, use `RUN_ARGS=--zap-log-level=2` for decisions. Unsafe body tracing
requires both `RUN_ARGS="--zap-log-level=10 --unsafe-exoscale-api-trace"`.

### Deploy a simple cluster
```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/cluster/
```

#### Wait for the workload cluster
```bash
$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=20m
$> ./bin/clusterctl get kubeconfig my-cluster > /tmp/my-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
$> kubectl wait cluster/my-cluster --for=condition=RemoteConnectionProbe --timeout=5m
$> kubectl wait machine/my-control-plane --for=condition=Ready --for=condition=Available --timeout=5m
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig get nodes

$> kubectl get exoscalecluster,machine,exoscalemachine,kubeadmconfig
```

### Delete simple cluster
```bash
$> kubectl delete cluster/my-cluster # also deletes its Machine and Exoscale resources
```

The shared `exoscale` credential Secret is not owned by the Cluster and remains
after Cluster deletion.

### Generate a workload cluster with clusterctl

As an alternative to the simple sample above, render the repository's
`KubeadmControlPlane` template with `clusterctl`:

```bash
$> export EXOSCALE_ZONE=ch-gva-2
$> ./bin/clusterctl generate cluster my-cluster \
     --from ./templates/cluster-template.yaml \
     --kubernetes-version v1.32.13 \
     | kubectl apply -f -

$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=20m
$> ./bin/clusterctl get kubeconfig my-cluster > /tmp/my-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig apply \
     -f https://github.com/flannel-io/flannel/releases/download/v0.28.8/kube-flannel.yml
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
$> kubectl delete cluster/my-cluster
```

## End-to-End testing
```bash
$> export EXOSCALE_API_KEY=<api-key>       # Optional if exocli is not configured
$> export EXOSCALE_API_SECRET=<api-secret> # Optional if exocli is not configured

$> make chainsaw-test-e2e CHAINSAW_MACHINE_TEMPLATE="Linux Ubuntu 24.04 LTS 64-bit"
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
