# cluster-api-provider-exoscale

## Prerequisites

- Install [`kubectl`][kubectl-install]
- Install [`minikube`][install-minikube]
- Install [`kustomize`][install-kustomize]

- set ENV variable to `GO111MODULE=off`

## Building, Running and Testing

### Start minikube and deploy the provider

```bash
minikube start
make deploy
```
then

```
go run cmd/manager/main.go
```

### Verify deployment

```bash
kubectl logs cluster-api-provider-exoscale-controller-manager-0 -n cluster-api-provider-exoscale-system  
```

## Clusterctl

```bash
go run cmd/clusterctl/main.go
```

[kubebuilder-book]: https://book.kubebuilder.io/
[install-dep]: https://github.com/golang/dep/blob/master/docs/installation.md
[kubectl-install]: http://kubernetes.io/docs/user-guide/prereqs/
[install-kustomize]: https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md
[install-minikube]: https://github.com/kubernetes/minikube#installation
