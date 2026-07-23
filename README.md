# Kubernetes Cluster API Provider Exoscale

## What is the Cluster API Provider Exoscale (CAPX)

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to
cluster creation, configuration and management.

CAPX is an infrastructure provider that provisions and manages Exoscale
resources for self-managed Kubernetes clusters. It currently reconciles
`ExoscaleCluster` and `ExoscaleMachine` resources into the required cloud
infrastructure.

## Run locally

### Deploy cluster api components
```Bash
$> kind create cluster --name capi-test
$> clusterctl init --infrastructure - # installs only CAPI core components
```

### Deploy cluster-api-provider-exoscale
```Bash
$> make generate manifests
$> make install run
```

### Deploy a simple cluster
```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/
```

#### Wait for the workload cluster
```bash
$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=20m
$> clusterctl get kubeconfig my-cluster > /tmp/my-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
$> kubectl wait cluster/my-cluster --for=condition=RemoteConnectionProbe --timeout=5m
$> kubectl wait machine/my-control-plane --for=condition=Ready --for=condition=Available --timeout=5m
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig get nodes

$> kubectl get exoscalecluster,machine,exoscalemachine,kubeadmconfig
```

### Delete simple cluster
```bash
$> kubectl delete cluster/my-cluster ## also deletes its Machine and Exoscale resources
```

The shared `exoscale` credential Secret is not owned by the Cluster and remains
after Cluster deletion.

## End-to-End testing
```Bash
$> export EXOSCALE_API_KEY=<api-key>       ## Optional if exocli is not configured
$> export EXOSCALE_API_SECRET=<api-secret> ## Optional if exocli is not configured

$> make chainsaw-test-e2e
```

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/cluster-api-provider-exoscale:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/cluster-api-provider-exoscale:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

### To Uninstall
Delete workload Clusters and wait for their cloud resources to be removed before
uninstalling CAPX.

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/cluster-api-provider-exoscale:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/cluster-api-provider-exoscale/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

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
