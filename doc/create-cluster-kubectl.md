# Use Exoscale ClusterAPI

## Create bootstrap cluster

### Use an existing k8s cluster or bootstrap it

With [kind](https://github.com/kubernetes-sigs/kind):
```
% kind create cluster --name=clusterapi
```
```
% export KUBECONFIG="$(kind get kubeconfig-path --name="clusterapi")"
```

## Deploy custom resources

Configuration is done via the following environement variables.

```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute
```

run `make`

```
% kubectl --kubeconfig=$KUBECONFIG \
            apply -f \
            provider-components.yaml
```
You can use the kubeconfig of your bootstrap cluster, `$KUBECONFIG` in our case with `kind`
 
## Deploy your cluster

use `cluster.yaml` and `machine.yaml` in the repo:
 - `cmd/clusterctl/examples/exoscale/`


```
% kubectl --kubeconfig=$KUBECONFIG \
            create -f \
            cmd/clusterctl/examples/exoscale/cluster.yaml
```

```
% kubectl --kubeconfig=$KUBECONFIG \
            create -f \
            cmd/clusterctl/examples/exoscale/machine.yaml
```

Follow the cluster bootstrap.

```console
% kubectl --kubeconfig=$KUBECONFIG \
            logs -f -n exoscale-provider-system \
            exoscale-provider-controllers-...
```

### have a look to your new cluster

Get your new `kubeconfig`from the new bootstrapped cluster
```
% ./bin/clusterctl --kubeconfig=$KUBECONFIG  \
             alpha phases  \
             get-kubeconfig \
             --provider exoscale \
             --cluster-name my-exoscale-cluster \
             --kubeconfig-out kubeconfig-newcluster
```

```
% kubectl --kubeconfig kubeconfig-newcluster get nodes
```
output
```
NAME                       STATUS    ROLES     AGE       VERSION
my-exoscale-master-k8crg   Ready     master    2m        v1.13.3
my-exoscale-node-7sz52     Ready     <none>    1m        v1.13.3
```
