<img src="https://user-images.githubusercontent.com/15922119/44146028-0dac3246-a08e-11e8-99dc-95c8731e9f3a.png" alt="Exoscale" align="right" height="120" width="120">
<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" alt="Exoscale" align="right" height="100" width="100">


# Kubernetes Cluster API Provider Exoscale


## Prerequisite

- [kubectl 1.14+](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- deepcopy-gen `$ go get k8s.io/code-generator/cmd/deepcopy-gen`

## Docker side

```console
% make docker-build

% make docker-push
```

## Run

Configuration is done via the following environement variables.


```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute
```

[kind](https://github.com/kubernetes-sigs/kind) is required to act as the bootstrap cluster. `run` calls `clusterctl`.

```
% go get -u sigs.k8s.io/kind

% make run
```

Follow the master bootstrap.

```console
% export KUBECONFIG=$(kind get kubeconfig-path --name=clusterapi)

% kubectl logs -f exoscale-provider-controller-... -n exoscale-provider-system
```

Follow the node bootstrap.

```console
% kubectl --kubeconfig kubeconfig logs -f exoscale-provider-controllers-... -n exoscale-provider-system
```

And finally.

```console
% kubectl --kubeconfig kubeconfig get nodes
NAME                       STATUS   ROLES    AGE     VERSION
my-exoscale-master-s5lmn   Ready    master   4m23s   v1.13.3
my-exoscale-node-n7dww     Ready    <none>   34s     v1.13.3
```

Deleting the cluster use `clusterctl` as well.

```console
% make delete
```


## Use Exoscale Cluster API

### Nodes example
- [Add a node to a deployed cluster](./doc/add-node-example.md)
- [Delete a node to a deployed cluster](./doc/delete-node-example.md)

### Create a cluster with `kubectl`
- [Cluster with two nodes example](./doc/create-cluster-kubectl.md)


