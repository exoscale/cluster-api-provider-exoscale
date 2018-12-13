# cluster-api-provider-exoscale

## Prerequisites

- Install [`dep`][install-dep]
- Install [`kubectl`][kubectl-install]
- Install [`minikube`][install-minikube]

## Building, Running and Testing

### Start minikube and deploy the provider

```bash
minikube start
make deploy
```

### Verify deployment

```bash
kubectl logs cluster-api-provider-exoscale-controller-manager-0 -n cluster-api-provider-exoscale-system  
```

[kubebuilder-book]: https://book.kubebuilder.io/
[install-dep]: https://github.com/golang/dep/blob/master/docs/installation.md
[kubectl-install]: http://kubernetes.io/docs/user-guide/prereqs/
[install-kustomize]: https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md
[install-minikube]: https://github.com/kubernetes/minikube#installation
