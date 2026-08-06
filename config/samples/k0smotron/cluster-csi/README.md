# Deploy the Exoscale CSI driver on a k0smotron cluster

This tutorial builds the same k0s cluster as the [cluster](../cluster)
tutorial, with two extra k0s flags that make it ready for the
[Exoscale CSI driver][exoscale-csi-driver], then walks through installing the
driver and provisioning a volume. Read the [cluster](../cluster) tutorial
first for how the CAPI/k0smotron pieces (`ExoscaleCluster`, `K0sControlPlane`,
`MachineDeployment`, ...) fit together. This README only calls out what's
different.

Prerequisites: Docker, `kind`, `kubectl` and Exoscale API credentials (see
[Permissions](#permissions) below for the CSI-specific role).

> **Run every command below from the root of the repository.**

## What's different from the base cluster

[control-plane.yaml](control-plane.yaml) and [workers.yaml](workers.yaml) add
two `k0s` args, applied to every node. Compared to the base cluster's
`K0sControlPlane`:

```diff
 apiVersion: controlplane.cluster.x-k8s.io/v1beta2
 kind: K0sControlPlane
 metadata:
   name: my-k0s-cluster-cp
   namespace: default
 spec:
   replicas: 1
   version: v1.36.3+k0s.0
   k0sConfigSpec:
     args:
       - --enable-worker
+      - --kubelet-root-dir=/var/lib/kubelet
+      - --labels=topology.kubernetes.io/zone=ch-gva-2,topology.kubernetes.io/region=ch-gva-2
```

And the base cluster's `K0sWorkerConfigTemplate`:

```diff
 apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
 kind: K0sWorkerConfigTemplate
 metadata:
   name: my-k0s-cluster-workers
   namespace: default
 spec:
   template:
     spec:
       version: v1.36.3+k0s.0
+      args:
+        - --kubelet-root-dir=/var/lib/kubelet
+        - --labels=topology.kubernetes.io/zone=ch-gva-2,topology.kubernetes.io/region=ch-gva-2
```

Before it can serve a single volume, the Exoscale CSI driver has to determine
which Exoscale instance it is running on and in which zone. It tries three
sources in order:
1. the Kubernetes `Node` object
2. the cloud-init config drive attached to the instance
3. the Exoscale metadata server over HTTP.
The first source needs two infromation: `spec.providerID` in the form
`exoscale://<instance-uuid>` (already set by this provider) and a
`topology.kubernetes.io/zone` label.

On a managed SKS cluster the Exoscale cloud-controller-manager supplies the label.
Running our own CCM here just for that is a lot of machinery for one field:
* a `Deployment` to install
* a second set of API credentials to manage
* `--cloud-provider=external` on every kubelet, which taints each node as uninitialized until the CCM gets round to clearing it.

Instead we set the label directly at node registration via k0s's
`--labels` flag: nothing extra runs in the cluster, and the driver's first
metadata source succeeds immediately. This is allowed without any privileged
component: `topology.kubernetes.io/zone` and `topology.kubernetes.io/region`
sit on the short allowlist of labels the `NodeRestriction` admission plugin
lets a kubelet set on itself. Being explicit here also stops the driver
falling through to the metadata server, which would make volume mounting
depend on cluster DNS and pod egress staying healthy.

`--kubelet-root-dir=/var/lib/kubelet` is unrelated to the labels above: k0s
defaults the kubelet root dir to `/var/lib/k0s/kubelet`, but the Exoscale CSI
driver's node plugin looks for the kubelet plugin registration/socket
directory at the standard `/var/lib/kubelet`. Moving it there is what lets
the driver find it.

## Deploy the k0s cluster

Same steps as the [cluster](../cluster) tutorial — see its README for what
each command does:
```bash
$> make clusterctl
$> kind create cluster --name capi-test
$> ./bin/clusterctl init --infrastructure - \
     --bootstrap k0sproject-k0smotron \
     --control-plane k0sproject-k0smotron
$> make generate manifests install
$> make run # keep the manager running, use another terminal below
```
```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/k0smotron/cluster-csi
$> kubectl wait cluster/my-k0s-cluster --for=condition=ControlPlaneInitialized --timeout=10m
$> kubectl get secret my-k0s-cluster-kubeconfig -o jsonpath='{.data.value}' | base64 -d > /tmp/my-k0s-cluster.kubeconfig
$> kubectl --kubeconfig=/tmp/my-k0s-cluster.kubeconfig wait node --all --for=condition=Ready --timeout=10m
```

## Deploy the Exoscale CSI driver

### Permissions

The CSI driver needs its own API key, separate from the one used above,
scoped to an IAM role with at least these permissions (see the
[driver's prerequisites][exoscale-csi-driver-prereq] for the up-to-date list):
```json
{
  "default-service-strategy": "deny",
  "services": {
    "compute": {
      "type": "rules",
      "rules": [
        {
          "expression": "operation in ['list-zones', 'get-block-storage-volume', 'list-block-storage-volumes', 'create-block-storage-volume', 'delete-block-storage-volume', 'attach-block-storage-volume-to-instance', 'detach-block-storage-volume', 'update-block-storage-volume-labels', 'resize-block-storage-volume', 'get-block-storage-snapshot', 'list-block-storage-snapshots', 'create-block-storage-snapshot', 'delete-block-storage-snapshot', 'list-quotas']",
          "action": "allow"
        }
      ]
    }
  }
}
```

### Create the credentials secret

```bash
$> export EXOSCALE_CSI_API_KEY=<api-key>
$> export EXOSCALE_CSI_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale-credentials \
    --namespace kube-system \
    --from-literal=EXOSCALE_API_KEY=$EXOSCALE_CSI_API_KEY \
    --from-literal=EXOSCALE_API_SECRET=$EXOSCALE_CSI_API_SECRET \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig
```

### Install the driver

```bash
$> export CSI_VERSION=0.34.3
$> kubectl --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    apply -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/main/deployment/${CSI_VERSION}/crds.yaml"
$> kubectl wait \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    --for=condition=Established \
    --timeout=60s \
    crd/volumesnapshotclasses.snapshot.storage.k8s.io \
    crd/volumesnapshotcontents.snapshot.storage.k8s.io \
    crd/volumesnapshots.snapshot.storage.k8s.io
$> kubectl --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    apply -k "github.com/exoscale/exoscale-csi-driver/deployment/${CSI_VERSION}?ref=main"
```

## Try it: a PVC-backed deployment

```bash
$> kubectl apply \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    -f https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/main/doc/examples/namespace.yaml
$> kubectl apply \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    -f https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/main/doc/examples/pvc.yaml
$> kubectl apply \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    -f https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/main/doc/examples/deployment.yaml

$> kubectl --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    wait deployment/my-awesome-deployment -n awesome --for=condition=Available --timeout=5m
$> kubectl --kubeconfig /tmp/my-k0s-cluster.kubeconfig get pods,pvc -n awesome

# You should get something like that
# NAME                                         READY   STATUS    RESTARTS   AGE
# pod/my-awesome-deployment-7786bb6547-xnr27   1/1     Running   0          48s
# 
# NAME                               STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS   AGE
# persistentvolumeclaim/my-sbs-pvc   Bound    pvc-45b893af-20b7-48a0-a67f-a30b1eecd277   200Gi      RWO            exoscale-sbs   <unset>                 54s
```

Check the Block Storage page of the [Exoscale portal][exoscale-portal] and
you'll see a volume matching the PVC's name.

## Clean up

```bash
$> kubectl delete \
    --kubeconfig /tmp/my-k0s-cluster.kubeconfig \
    -f https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/main/doc/examples/namespace.yaml
```

This deletes the PVC and its backing volume, but leaves the k0s cluster
itself running. To tear down the whole cluster:
```bash
$> kubectl delete cluster/my-k0s-cluster # also deletes its Machines and Exoscale resources
```

[exoscale-csi-driver]: https://github.com/exoscale/exoscale-csi-driver
[exoscale-csi-driver-prereq]: https://github.com/exoscale/exoscale-csi-driver#prerequisite
[exoscale-portal]: https://portal.exoscale.com
