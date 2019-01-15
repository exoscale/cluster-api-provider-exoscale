# cluster-api-provider-exoscale



```console
% minikube start --kubernetes-version v1.12.4 --vm-driver kvm2

% eval $(minikube docker-env)

% make docker-build
```

```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute
```

```console
% make install
```

```console
% go run cmd/clusterctl/main.go create cluster \
        --provider exoscale \
        -m cmd/clusterctl/examples/exoscale/machine.yaml \
        -c cmd/clusterctl/examples/exoscale/cluster.yaml \
        -p provider-components.yaml \
        -e ~/.kube/config
```
