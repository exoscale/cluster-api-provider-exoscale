# Use Exoscale ClusterAPI

## Installation

### Follow the instructions on

[cluster-api-provider-exoscale](https://github.com/pierre-emmanuelJ/cluster-api-provider-exoscale#cluster-api-provider-exoscale)

## Delete a node to a deployed cluster

have a look to your nodes on your deployed cluster:
```
% kubectl --kubeconfig kubeconfig get nodes
my-exoscale-master-jg4dw   Ready    master   2m   v1.12.5
my-exoscale-node-5sxj9     Ready    <none>   1m   v1.12.5
```

## Delete a node using cluster API

```
% kubectl --kubeconfig=kubeconfig \
            delete machines.cluster.k8s.io \
            my-exoscale-node-5sxj9
```
output
```
machine.cluster.k8s.io "my-exoscale-node-5sxj9" deleted
```

Follow the node deletation.

```console
% kubectl --kubeconfig kubeconfig \
            logs -f exoscale-provider-controllers-... \
            -n exoscale-provider-system
```

### have a look to your nodes

```
% kubectl --kubeconfig kubeconfig get nodes
my-exoscale-master-jg4dw   Ready    master   3m   v1.12.5
```