# cluster-api-provider-exoscale

Spawn a fresh kubernetes cluster, feel free to delete any old one if something looks funny.

```console
% minikube start --kubernetes-version v1.12.4 --vm-driver kvm2
```

## hacking the clusterctl side

Building the `manager` image.

```
% eval $(minikube docker-env)

% make docker-build
```

Build the manifests.


```console
export EXOSCALE_API_KEY=EXO...
export EXOSCALE_SECRET_KEY=...
export EXOSCALE_COMPUTE_ENDPOINT=https://api.exoscale.com/compute

% make deploy
```

Run the `clusterctl` command.

```console
% go run cmd/clusterctl/main.go create cluster \
        --provider exoscale \
        -m cmd/clusterctl/examples/exoscale/machine.yaml \
        -c cmd/clusterctl/examples/exoscale/cluster.yaml \
        -p provider-components.yaml \
        -e ~/.kube/config
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
% go run cmd/manager/main.go
```
