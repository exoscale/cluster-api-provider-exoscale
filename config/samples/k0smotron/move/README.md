# Move a workload cluster with `clusterctl move`

`clusterctl move` transfers the Cluster API objects of a workload cluster from
one management cluster to another. The workload cluster itself is never
touched: its nodes keep running, the Exoscale instances are neither recreated
nor rebooted, only *who reconciles them* changes. That's how you migrate off a
temporary bootstrap cluster onto a permanent management cluster.

This guide uses two local `kind` clusters, `capi-source` and `capi-target`. It
deploys one k0s workload cluster on the source with the manifests in
[../cluster](../cluster), then moves it to the target.

Prerequisites: Docker, `kind`, `kubectl`, `curl`, `jq` and Exoscale API credentials.

> **Run every command below from the root of the repository.**

## Part 1 — Source management cluster and workload cluster

This part is only the starting point for the move. It is the tutorial in
[../cluster/README.md](../cluster/README.md) condensed — read that one for what
each manifest contains, the security group rules, and how to scale the cluster.

```bash
$> make clusterctl
$> kind create cluster --name capi-source
$> kind get kubeconfig --name capi-source > /tmp/source.kubeconfig
```

Install CAPI, the Exoscale infrastructure provider and the k0smotron providers
on it. Check [Installation](../../../../README.md#installation) to configure
`clusterctl`. The Exoscale provider is pinned to its latest release, so the
target can be installed at the exact same version in Part 2:

```bash
$> export EXOSCALE_PROVIDER_VERSION=$(curl -s https://api.github.com/repos/exoscale/cluster-api-provider-exoscale/releases/latest | jq -r .tag_name)
$> ./bin/clusterctl init \
     --kubeconfig /tmp/source.kubeconfig \
     --core cluster-api:v1.14.2 \
     --infrastructure exoscale:$EXOSCALE_PROVIDER_VERSION \
     --bootstrap k0sproject-k0smotron:v2.1.1 \
     --control-plane k0sproject-k0smotron:v2.1.1
```

Create the credentials Secret and deploy the cluster:

```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl --kubeconfig /tmp/source.kubeconfig create secret generic exoscale \
     --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl --kubeconfig /tmp/source.kubeconfig apply -k config/samples/k0smotron/cluster
```

Wait until the cluster is fully provisioned — 1 control-plane node and 1 worker:

```bash
$> kubectl --kubeconfig /tmp/source.kubeconfig wait cluster/my-k0s-cluster --for=condition=RemoteConnectionProbe --timeout=10m
$> kubectl --kubeconfig /tmp/source.kubeconfig wait k0scontrolplane/my-k0s-cluster-cp --for=jsonpath='{.status.readyReplicas}'=1 --timeout=10m
$> kubectl --kubeconfig /tmp/source.kubeconfig wait machinedeployment/my-k0s-cluster-workers --for=jsonpath='{.status.readyReplicas}'=1 --timeout=10m
```

Move a cluster that is still provisioning and you won't be able to tell a move
problem from a provisioning problem, so don't skip this wait.

## Part 2 — Target management cluster

The target needs the same providers at the same versions and its own
credentials Secret.

### 1. Create it and export its kubeconfig

```bash
$> kind create cluster --name capi-target
$> kind get kubeconfig --name capi-target > /tmp/target.kubeconfig
```

### 2. Install the same providers

Every provider installed on the source must also exist on the target, at a
version greater than or equal to the source's — `clusterctl move` checks this and
refuses to start otherwise. This shows what the source runs:

```bash
$> kubectl --kubeconfig /tmp/source.kubeconfig get providers -A
```

Install the same set on the target — the same versions as Part 1:

```bash
$> ./bin/clusterctl init \
     --kubeconfig /tmp/target.kubeconfig \
     --core cluster-api:v1.14.2 \
     --infrastructure exoscale:$EXOSCALE_PROVIDER_VERSION \
     --bootstrap k0sproject-k0smotron:v2.1.1 \
     --control-plane k0sproject-k0smotron:v2.1.1
```

### 3. Recreate the Secret

```bash
$> kubectl --kubeconfig /tmp/target.kubeconfig create secret generic exoscale \
     --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
```

The Secret has to be recreated by hand because it is shared, not owned by the
`Cluster`, and `move` only carries objects belonging to the cluster being moved.

## Part 3 — The move

Dry-run first. It performs the discovery step without writing anything:

```bash
$> ./bin/clusterctl move --kubeconfig /tmp/source.kubeconfig \
     --to-kubeconfig /tmp/target.kubeconfig --dry-run -v 5
```

Check the discovered objects include this provider's kinds — `ExoscaleCluster`,
`ExoscaleMachine` ×2 and `ExoscaleMachineTemplate` ×2 — alongside the CAPI and
k0smotron ones. For the sample cluster that is 23 objects in total:

```
Discovering Cluster API objects
ExoscaleMachineTemplate count=2
ExoscaleCluster         count=1
ExoscaleMachine         count=2
Machine                 count=2
Cluster                 count=1
[...]
```

A dry run skips the provider check of Part 2 step 2 entirely, so it passing tells
you nothing about whether the target is correctly initialized.

Then run it for real:

```bash
$> ./bin/clusterctl move --kubeconfig /tmp/source.kubeconfig \
     --to-kubeconfig /tmp/target.kubeconfig -v 5
```

It pauses the source `Cluster` (`Set Cluster.Spec.Paused paused=true`), creates
every object on the target, deletes them from the source, and finally unpauses
on the target (`paused=false`). While paused, the Exoscale provider on both
management clusters logs
`InfraCluster is paused, skipping reconciliation` — that is the pause doing its
job, not an error.

### Verify

```bash
$> ./bin/clusterctl describe cluster my-k0s-cluster --kubeconfig /tmp/target.kubeconfig
$> kubectl --kubeconfig /tmp/source.kubeconfig get cluster,exoscalecluster,machine,exoscalemachine
No resources found
```

The real test is that the target now *owns* reconciliation, so scale the worker
pool there and watch the new node come up:

```bash
$> kubectl --kubeconfig /tmp/target.kubeconfig patch machinedeployment my-k0s-cluster-workers --type=merge -p '{"spec":{"replicas":2}}'
$> kubectl --kubeconfig /tmp/target.kubeconfig wait machinedeployment/my-k0s-cluster-workers --for=jsonpath='{.status.readyReplicas}'=2 --timeout=10m
```

Finally confirm in the [Exoscale console][exoscale-console] that no
infrastructure was duplicated: still one Elastic IP, two security groups, and
one instance per `Machine`.

## Clean up

Delete the workload cluster from whichever management cluster owns it — the
target, once the move is done:

```bash
$> kubectl --kubeconfig /tmp/target.kubeconfig delete cluster/my-k0s-cluster
$> kind delete cluster --name capi-source
$> kind delete cluster --name capi-target
```

Deleting the `Cluster` cascades to its `Machine`s and all Exoscale resources.
Delete the kind clusters *after* the workload cluster: drop the management
cluster first and nothing is left to run the finalizers, leaving orphaned
Exoscale resources to clean up by hand.

[exoscale-console]: https://portal.exoscale.com
