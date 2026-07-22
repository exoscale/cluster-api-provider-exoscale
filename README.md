# Cluster API Provider Exoscale

Kubernetes-native declarative infrastructure for Exoscale.

## What is the Cluster API Provider Exoscale?

The Cluster API Provider Exoscale (CAPX) is an infrastructure provider for
[Cluster API](https://cluster-api.sigs.k8s.io/). It provides declarative APIs
for provisioning and managing the Exoscale infrastructure used by self-managed
Kubernetes clusters.

CAPX reconciles `ExoscaleCluster` and `ExoscaleMachine` resources into Exoscale
Elastic IPs, security groups, and Compute instances. Cluster API bootstrap and
control-plane providers manage the Kubernetes lifecycle on top of that
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
```Bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET

$> cat <<EOF | kubectl apply -f -
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ExoscaleCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  zone: ch-gva-2
  securityGroupControlPlane:
    rules:
      - description: allow ssh
        network: "0.0.0.0/0"
        endPort: 22
        startPort: 22
        protocol: tcp
        flowDirection: ingress
  securityGroupNode:
    rules:
      - description: allow ssh
        network: "0.0.0.0/0"
        endPort: 22
        startPort: 22
        protocol: tcp
        flowDirection: ingress
EOF

$> cat <<EOF | kubectl apply -f -
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
        - 10.244.0.0/16
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: ExoscaleCluster
    name: my-cluster
EOF

$> cat <<'EOF' | kubectl apply -f -
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfig
metadata:
  name: my-control-plane
  namespace: default
spec:
  format: cloud-config
  preKubeadmCommands:
    - |
      set -eux
      swapoff -a

      cat >/etc/modules-load.d/kubernetes.conf <<EOF
      overlay
      br_netfilter
      EOF
      modprobe overlay
      modprobe br_netfilter

      cat >/etc/sysctl.d/99-kubernetes.conf <<EOF
      net.bridge.bridge-nf-call-iptables = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward = 1
      EOF
      sysctl --system

      apt-get update
      apt-get install -y ca-certificates curl gpg containerd

      install -m 0755 -d /etc/apt/keyrings
      curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.32/deb/Release.key \
        | gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
      echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.32/deb/ /' \
        >/etc/apt/sources.list.d/kubernetes.list

      apt-get update
      apt-get install -y \
        kubeadm=1.32.13-1.1 \
        kubelet=1.32.13-1.1 \
        kubectl=1.32.13-1.1
      apt-mark hold kubeadm kubelet kubectl

      mkdir -p /etc/containerd
      containerd config default >/etc/containerd/config.toml
      sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
      echo "KUBELET_EXTRA_ARGS=--provider-id=exoscale://$(cat /var/lib/cloud/data/instance-id)" \
        >/etc/default/kubelet
      systemctl enable --now containerd
      systemctl enable kubelet
  postKubeadmCommands:
    - kubectl --kubeconfig=/etc/kubernetes/admin.conf apply -f https://github.com/flannel-io/flannel/releases/download/v0.28.8/kube-flannel.yml
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ExoscaleMachine
metadata:
  name: my-control-plane
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
spec:
  template: Linux Ubuntu 24.04 LTS 64-bit
  instanceType: small
  rootVolumeSizeGiB: 20
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: my-control-plane
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
    cluster.x-k8s.io/control-plane: ""
spec:
  clusterName: my-cluster
  version: v1.32.13
  bootstrap:
    configRef:
      apiGroup: bootstrap.cluster.x-k8s.io
      kind: KubeadmConfig
      name: my-control-plane
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: ExoscaleMachine
    name: my-control-plane
EOF

## Wait for the workload control plane and node to become ready.

$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=20m
$> clusterctl get kubeconfig my-cluster > /tmp/my-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
$> kubectl --kubeconfig=/tmp/my-cluster.kubeconfig get nodes

$> kubectl get cluster,exoscalecluster,machine,exoscalemachine,kubeadmconfig
```

### Delete simple cluster
```Bash
$> kubectl delete cluster/my-cluster ## also deletes its Machine and Exoscale resources
```

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

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

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
