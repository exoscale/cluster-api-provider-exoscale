# cluster-api-provider-exoscale



```console
% minikube start --kubernetes-version v1.12.4 --vm-driver kvm2
```

% eval $(minikube docker-env)

% make docker-build
```

```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute
```

```console
% make manifests
```

```console
% go run cmd/clusterctl/main.go create cluster \
        --provider exoscale \
        -m cmd/clusterctl/examples/exoscale/machine.yml \
        -c cmd/clusterctl/examples/exoscale/cluter.yaml \
        -p provider-components.yaml \
        --vm-driver kvm2 \
        --minikube kubernetes-version=1.12.4
```
