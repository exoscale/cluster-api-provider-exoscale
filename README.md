# Kubernetes Cluster API Provider Exoscale

## Introduction

The [Cluster API][cluster_api] project brings declarative, Kubernetes-style
APIs to cluster creation, configuration, and management. The Cluster API
Provider Exoscale (CAPEX) is the infrastructure provider: it turns its custom
resources into Exoscale cloud resources. Turning those cloud resources into a
running Kubernetes cluster is the job of the other kind of
[Cluster API provider][capi-providers], a bootstrap/control-plane provider —
see the samples below for two examples.

It reconciles four custom resources:

| Kind | Purpose |
| --- | --- |
| `ExoscaleCluster` | Cluster-wide infrastructure: the Exoscale zone, the control-plane Elastic IP, and the control-plane/worker security groups. |
| `ExoscaleMachine` | A single Exoscale Compute Instance backing a Cluster API `Machine`. |
| `ExoscaleClusterTemplate` | A reusable `ExoscaleCluster` template, e.g. for `ClusterClass`. |
| `ExoscaleMachineTemplate` | A reusable `ExoscaleMachine` template, cloned by control-plane providers and `MachineDeployment`s to create `ExoscaleMachine`s at scale. |

## Samples

### kubeadm

[kubeadm][kubeadm] is the default provider, installed by the Cluster API CLI
itself. It expects Kubernetes (kubeadm, kubelet, kubectl, containerd) to
already be installed on the machine before it configures the node.

- [cluster](config/samples/kubeadm/cluster/README.md): a simple cluster whose control
  plane can be scaled up. Kubernetes is installed at boot time by cloud-init
  running plain `apt-get`/`systemctl` commands — a quick way to try things
  out, not a production-grade way to provision nodes.
- [cluster-custom-image](config/samples/kubeadm/cluster-custom-image/README.md):
  the same cluster, built from a custom Linux template with Kubernetes
  pre-baked in — because kubeadm expects every Kubernetes component to
  already be present on the machines the infrastructure provider spins up.

### k0smotron

[k0smotron][k0smotron] backs the cluster with [k0s][k0s] and needs no pre-baked/custom Linux template: it installs Kubernetes itself.

- [cluster](config/samples/k0smotron/cluster/README.md): a simple cluster
  that can scale its control plane and worker nodes independently.
- [cluster-csi](config/samples/k0smotron/cluster-csi/README.md): the same
  cluster with the [Exoscale CSI driver][exoscale-csi-driver] installed, to
  create volumes.

## Install a released provider

Prerequisites: `clusterctl`, `kubectl`, a management cluster, and a GitHub
token that can read this private repository. A fine-grained token needs
`Contents: read`. A classic token needs the `repo` scope.

Pin the release in both the repository URL and the `clusterctl` command:

```bash
$> export GITHUB_TOKEN=<github-token>
$> export GOPROXY=off
$> export CAPX_VERSION=v0.1.0-alpha.1
$> cat >/tmp/clusterctl-exoscale.yaml <<EOF
providers:
- name: exoscale
  url: https://github.com/exoscale/cluster-api-provider-exoscale/releases/${CAPX_VERSION}/infrastructure-components.yaml
  type: InfrastructureProvider
EOF
$> clusterctl init \
     --config /tmp/clusterctl-exoscale.yaml \
     --infrastructure "exoscale:${CAPX_VERSION}"
```

`GOPROXY=off` makes `clusterctl` download the private release directly from
GitHub with `GITHUB_TOKEN`. The samples add their required bootstrap and
control-plane providers to the same command.

## Local development

Prerequisites: `go`, `docker`, `kind`, `kubectl`, `make` and Exoscale API credentials.

### Run the provider locally

Create a kind cluster and install Cluster API with no infrastructure
provider — the manager started by `make run` below acts as one, reconciling
`ExoscaleCluster`/`ExoscaleMachine` against this cluster from your host:
```bash
$> make clusterctl
$> kind create cluster --name capi-test
$> CAPI_VERSION=$(go list -m -f '{{.Version}}' sigs.k8s.io/cluster-api)
$> ./bin/clusterctl init \
     --core "cluster-api:$CAPI_VERSION" \
     --bootstrap "kubeadm:$CAPI_VERSION" \
     --control-plane "kubeadm:$CAPI_VERSION" \
     --infrastructure -
```

```bash
$> make generate manifests install
$> make run
```

### Run the end-to-end tests

Unlike the steps above, `make chainsaw-test-e2e` is self-contained: it builds
the controller image, creates its own dedicated Kind cluster, installs
Cluster API with the k0smotron bootstrap/control-plane providers, and deploys
the built image, then runs the chainsaw tests under `test/chainsaw/`.
```bash
$> export EXOSCALE_API_KEY=<api-key>       # Optional if exocli is configured
$> export EXOSCALE_API_SECRET=<api-secret> # Optional if exocli is configured

# run every e2e tests
$> make chainsaw-test-e2e

# run specific tests
$> make chainsaw-test-e2e CHAINSAW_TEST_DIRS="test/chainsaw/full-deployment test/chainsaw/cluster-webhook"
```

Tear down the dedicated Kind cluster once you're done:
```bash
$> make cleanup-test-e2e
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
[exoscale]: https://www.exoscale.com/
[capi-providers]: https://cluster-api.sigs.k8s.io/user/concepts#providers
[kubeadm]: https://cluster-api.sigs.k8s.io/tasks/bootstrap/kubeadm-bootstrap
[k0smotron]: https://docs.k0smotron.io
[k0s]: https://k0sproject.io/
[exoscale-csi-driver]: https://github.com/exoscale/exoscale-csi-driver
