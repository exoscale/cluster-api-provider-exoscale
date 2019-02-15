# Use Exoscale ClusterAPI

## Installation

### Follow the instructions on

[cluster-api-provider-exoscale](https://github.com/pierre-emmanuelJ/cluster-api-provider-exoscale#cluster-api-provider-exoscale)

## Now you have a Kubernetes cluster deployed

have a look to your nodes:
```
% kubectl --kubeconfig kubeconfig get nodes
my-exoscale-master-jg4dw   Ready    master   2m   v1.12.5
my-exoscale-node-5sxj9     Ready    <none>   1m   v1.12.5
```

## Add a node using cluster API

use `machine-node.yaml.example` in the repo:
 - `cmd/clusterctl/examples/exoscale/machine-node.yaml.example`



```
% kubectl --kubeconfig=kubeconfig \
            create -f \
            cmd/clusterctl/examples/exoscale/machine-node.yaml
```
output
```
machine.cluster.k8s.io/my-exoscale-node-klhqp created
```

Follow the node bootstrap.

```console
% kubectl --kubeconfig kubeconfig \
            logs -f exoscale-provider-controllers-... \
            -n exoscale-provider-system
```

### have a look to your new node

have a look to your nodes:
```
% kubectl --kubeconfig kubeconfig get nodes
my-exoscale-master-jg4dw   Ready    master   3m   v1.12.5
my-exoscale-node-5sxj9     Ready    <none>   2m   v1.12.5
my-exoscale-node-7fgx3     Ready    <none>   1m   v1.12.5
```