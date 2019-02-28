<img src="https://user-images.githubusercontent.com/15922119/44146028-0dac3246-a08e-11e8-99dc-95c8731e9f3a.png" alt="Exoscale" align="right" height="120" width="120">
<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" alt="Exoscale" align="right" height="100" width="100">


# Kubernetes Cluster API Provider Exoscale


## docker side

```diff
--- a/Makefile
+++ b/Makefile
-PREFIX = exoscale
+PREFIX = greut
```

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

[kind](https://github.com/kubernetes-sigs/kind) is required to act as the bootstrap cluster.

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

Clean up by deleting the data from the CRDs before removing the other resources.

```console
% kubectl delete machines.cluster.k8s.io my-exoscale-...
% kubectl delete clusters.cluster.k8s.io my-exoscale-...
% kubectl delete -f provider-components.yaml
```


## Use Exoscale Cluster API

### nodes example
- [add a node to a deployed cluster](./doc/add-node-example.md)
- [delete a node to a deployed cluster](./doc/delete-node-example.md)
