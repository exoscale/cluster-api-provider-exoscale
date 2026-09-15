# Deploy a cluster with kubeadm

This sample uses Cluster API's kubeadm bootstrap and control-plane providers to
create one control-plane Node and one worker Node on Exoscale. The control plane
uses `KubeadmControlPlane`; the worker pool uses a `MachineDeployment` and
`KubeadmConfigTemplate`. Both use `ExoscaleMachineTemplate` for the underlying
VMs.

Prerequisites: Docker, `kind`, `kubectl`, `make`, the Go version declared in
`go.mod`, and Exoscale API credentials.

Run every command below from the root of the repository.

## Why the sample bootstraps the stock image

The stock Ubuntu template does not contain Kubernetes. The
`preKubeadmCommands` in [cluster.yaml] prepare each control-plane and worker VM
before kubeadm runs:

- disable swap, which kubelet does not support by default
- load the kernel modules and set the sysctls needed for container networking
- install and configure containerd with the systemd cgroup driver
- install kubeadm, kubelet, and kubectl from the Kubernetes package repository
- set `exoscale://<instance-uuid>` as the kubelet provider ID so Cluster API can
  match the Node to its `Machine` and `ExoscaleMachine`

The commands appear twice because control-plane and worker Machines use
separate kubeadm bootstrap resources. The [pre-built image sample] replaces
both copies with only the per-instance provider-ID command.

This sample pins Kubernetes `v1.32.13`. The apt repository series, package
versions, `KubeadmControlPlane.spec.version`, and
`MachineDeployment.spec.template.spec.version` must stay aligned. The `-1.1`
suffix in the apt package versions is the Debian package revision, not part of
the Kubernetes version.

Kubeadm does not install a Container Network Interface plugin. The
`postKubeadmCommands` therefore install Flannel after the control plane starts.
Flannel uses `10.244.0.0/16`, which is why the same cidr is declared in
`Cluster.spec.clusterNetwork.pods.cidrBlocks`. Without a CNI, pods cannot
communicate across Nodes and CoreDNS does not become ready.

## Deploy Cluster API components

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

## Run Exoscale CAPI

```bash
$> make generate manifests install
$> make run
```

Keep the manager running and use another terminal for the remaining commands.

## Deploy the workload cluster

```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/kubeadm/cluster/
```

### Wait for the workload cluster

```bash
$> kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=10m
$> WORKLOAD_KUBECONFIG=$(mktemp /tmp/my-cluster.kubeconfig.XXXXXX)
$> ./bin/clusterctl get kubeconfig my-cluster > "$WORKLOAD_KUBECONFIG"
$> kubectl wait cluster/my-cluster --for=condition=RemoteConnectionProbe --timeout=5m
$> kubectl wait kubeadmcontrolplane/my-control-plane --for=condition=Available --timeout=5m
$> kubectl wait machinedeployment/my-workers --for=condition=Available --timeout=15m
$> kubectl wait machine --selector='cluster.x-k8s.io/cluster-name=my-cluster' --for=condition=Ready --timeout=5m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" wait node --all --for=condition=Ready --timeout=10m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get nodes

$> kubectl get cluster,kubeadmcontrolplane,machinedeployment,exoscalecluster,exoscalemachinetemplate,machine,exoscalemachine,kubeadmconfig,kubeadmconfigtemplate
```

## Test workload networking

A ready Node only proves that kubelet is reporting health. This DaemonSet runs
one nginx pod on every worker, while the default control-plane taint keeps it
off control-plane Nodes. Reaching it through a Service also checks pod
scheduling, DNS, Service routing, and the Flannel network.

```bash
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" apply -f - <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: smoke-test
spec:
  selector:
    matchLabels:
      app: smoke-test
  template:
    metadata:
      labels:
        app: smoke-test
    spec:
      containers:
        - name: nginx
          image: nginx:1.29-alpine
---
apiVersion: v1
kind: Service
metadata:
  name: smoke-test
spec:
  selector:
    app: smoke-test
  ports:
    - port: 80
EOF
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" rollout status daemonset/smoke-test --timeout=5m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get pods -l app=smoke-test -o wide
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" run curl --rm --attach --restart=Never --image=curlimages/curl:8.16.0 -- curl -fsS http://smoke-test
```

## Scale the cluster

Scale the worker pool from one to three Nodes. The DaemonSet automatically
creates one smoke-test pod on each new worker:

```bash
$> kubectl patch machinedeployment my-workers --type=merge -p '{"spec":{"replicas":3}}'
$> kubectl wait machinedeployment/my-workers --for=jsonpath='{.status.readyReplicas}'=3 --timeout=10m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" rollout status daemonset/smoke-test --timeout=5m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get pods -l app=smoke-test -o wide
```

Scale the control plane from one to three Nodes. `ExoscaleCluster` provides one
Elastic IP as the Kubernetes API endpoint and CAPX attaches it to every
control-plane instance, so clients continue to use the same address:

```bash
$> kubectl patch kubeadmcontrolplane my-control-plane --type=merge -p '{"spec":{"replicas":3}}'
$> kubectl wait kubeadmcontrolplane/my-control-plane --for=jsonpath='{.status.readyReplicas}'=3 --timeout=10m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" wait node --all --for=condition=Ready --timeout=10m
```

Patch both resources back to one replica when the scaling check is complete.
Keep at least one control-plane replica:

```bash
$> kubectl patch machinedeployment my-workers --type=merge -p '{"spec":{"replicas":1}}'
$> kubectl wait machinedeployment/my-workers --for=jsonpath='{.status.readyReplicas}'=1 --timeout=10m
$> kubectl patch kubeadmcontrolplane my-control-plane --type=merge -p '{"spec":{"replicas":1}}'
$> kubectl wait kubeadmcontrolplane/my-control-plane --for=jsonpath='{.status.readyReplicas}'=1 --timeout=10m
```

## Delete the cluster

```bash
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" delete daemonset,service smoke-test
$> kubectl delete cluster/my-cluster
$> rm -f "$WORKLOAD_KUBECONFIG"
```

Deleting the `Cluster` also deletes its Machines and Exoscale resources. The
shared `exoscale` credential Secret is not owned by the Cluster and remains.

[cluster.yaml]: cluster.yaml
[pre-built image sample]: ../cluster-custom-image/README.md
