# Kubernetes Cluster API Provider Exoscale

## Introduction

The [Cluster API][cluster_api] brings declarative, Kubernetes-style APIs to
cluster creation, configuration and management.

Exoscale CAPI is an infrastructure provider that provisions and manages Exoscale
resources for self-managed Kubernetes clusters. It currently reconciles
`ExoscaleCluster` and `ExoscaleMachine` resources into the required cloud
infrastructure.

## Getting started

Choose a sample based on the bootstrap and control-plane provider you want to
use. Each guide covers prerequisites, local setup, deployment, verification,
scaling, and cleanup.

- [kubeadm cluster]: install Kubernetes on the stock Ubuntu template, deploy a
  control plane and worker pool, test networking, and scale both pools.
- [kubeadm cluster with a pre-built image]: build and register a private
  template with Kubernetes already installed.
- [k0smotron cluster]: deploy a machine-based k0s control plane and workers.
- [k0smotron cluster with CSI]: install the Exoscale CSI driver and provision a
  block storage volume.

## Development

Install the CRDs in the current Kubernetes context and run the controller from
your host:

```bash
$> make generate manifests install
$> make run
```

Run the unit tests and linters:

```bash
$> make test
$> make lint-config
$> make lint
```

### End-to-End testing

```bash
$> export EXOSCALE_API_KEY=<api-key>       # Optional if exocli is not configured
$> export EXOSCALE_API_SECRET=<api-secret> # Optional if exocli is not configured

# run every e2e tests
$> make chainsaw-test-e2e

# run specific tests
$> make chainsaw-test-e2e CHAINSAW_TEST_DIRS="test/chainsaw/full-deployment test/chainsaw/cluster-webhook"
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
[k0smotron cluster]: config/samples/k0smotron/cluster/README.md
[k0smotron cluster with CSI]: config/samples/k0smotron/cluster-csi/README.md
[kubeadm cluster]: config/samples/kubeadm/cluster/README.md
[kubeadm cluster with a pre-built image]: config/samples/kubeadm/cluster-custom-image/README.md
