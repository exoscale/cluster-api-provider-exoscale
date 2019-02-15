<img src="https://user-images.githubusercontent.com/15922119/44146028-0dac3246-a08e-11e8-99dc-95c8731e9f3a.png" alt="Exoscale" align="right" height="120" width="120">
<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" alt="Exoscale" align="right" height="100" width="100">


# Kubernetes Cluster API Provider Exoscale

Spawn a fresh kubernetes cluster, feel free to delete any old one if something looks funny.

```console
% minikube start --kubernetes-version v1.12.5 --vm-driver kvm2
```

## clusterctl side

Build the manifests.

```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute

% make deploy
```

Run the `clusterctl` command.

```console
% go run cmd/clusterctl/main.go create cluster -v 9 \
        --provider exoscale \
        -m cmd/clusterctl/examples/exoscale/machine.yaml \
        -c cmd/clusterctl/examples/exoscale/cluster.yaml \
        -p provider-components.yaml \
        -e ~/.kube/config
```

Follow the master bootstrap.

```console
% kubectl logs -f exoscale-provider-controllers-... -n exoscale-provider-system
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



## hacking the manager side

By default, the manager is run as a container. Let's run it manually instead.

```diff
 resources:
-- ../manager/manager.yaml
+#- ../manager/manager.yaml

 patchesStrategicMerge:
-- manager_image_patch.yaml
+#- manager_image_patch.yaml
```

Same as above.

```console
% make deploy
```

```console
% go run cmd/manager/main.go -v 9
```

**WARNING**, doing so means the workers step cannot be completed. You'll only get the master nodes.

## Using [KIND](https://github.com/kubernetes-sigs/kind)

This is highly experimental...

- https://github.com/kubernetes-sigs/cluster-api/pull/710


```console
% go run cmd/clusterctl/main.go create cluster -v 9 \
        --provider exoscale \
        -m cmd/clusterctl/examples/exoscale/machine.yaml \
        -c cmd/clusterctl/examples/exoscale/cluster.yaml \
        -p provider-components.yaml \
        --bootstrap-type kind \
        --bootstrap-flags "image=kindest/node:v1.12.3" \
        --bootstrap-flags "config=kind-config.yaml" \
        --bootstrap-flags "loglevel=debug"
```

...

```yaml
kind: Config
apiVersion: kind.sigs.k8s.io/v1alpha2
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    apiVersion: kubeadm.k8s.io/v1alpha3
    kind: ClusterConfiguration
    networking:
      serviceSubnet: 10.0.0.0/16
    kubernetesVersion: v1.12.3
  kubeadmConfigPatchesJson6902:
  - group: kubeadm.k8s.io
    version: v1alpha3
    kind: ClusterConfiguration
    patch: |
      - op: add
        path: /apiServerCertSANs/-
        value: localhost
- role: worker
  replicas: 1
```

**NB** kubeadm v1.12 is `v1alpha3` when v1.13 is `v1beta`



## Use Exoscale Cluster API

### nodes example
- [add a node to a deployed cluster](./doc/add-node-example.md)
