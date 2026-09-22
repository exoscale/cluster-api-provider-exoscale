# Pre-built Kubernetes image

This overlay runs the [kubeadm cluster sample] with Kubernetes pre-installed
in a private Exoscale template. It uses the generic Ubuntu 24.04 UEFI QEMU
target from [Kubernetes image-builder]. No Exoscale-specific image-builder
target is required.

The resulting amd64 image was built with image-builder `v0.1.55` and contains
Ubuntu `24.04.4`, Kubernetes `v1.34.11`, and containerd `2.3.2`.

## Image requirements

The image must include cloud-init with the Exoscale datasource, containerd,
kubeadm, kubelet, and kubectl. The Kubernetes version baked into the image must
match the `KubeadmControlPlane` and `MachineDeployment` versions patched in
`kustomization.yaml`.

The provider ID cannot be baked into the image because it contains the new VM's
UUID. This overlay therefore removes package installation from cloud-init but
keeps the per-instance provider-ID command. The same command labels every Node
with its Exoscale zone and region so the [Exoscale CSI driver] can identify it
without falling back to the metadata server.

The zone and region are hardcoded to `ch-gva-2`, matching
`ExoscaleCluster.spec.zone` in the base sample. Change all three values together
when using another zone. Kubeadm already uses the standard `/var/lib/kubelet`
root directory expected by the CSI driver.

The overlay replaces the base `preKubeadmCommands` with the provider-ID and
label command. It keeps the inherited `postKubeadmCommands`, which installs
Flannel after kubeadm.

## Create the image

Requirements: Podman, KVM exposed as `/dev/kvm`, at least 15 GiB of free disk,
`qemu-img`, `jq`, and the Exoscale CLI.

The tested first build took about 30 minutes and downloaded a 3.4 GiB Ubuntu
ISO. Keep `packer-cache` between builds so the ISO can be reused.

Create a working directory and an image-builder version override:

```bash
$ mkdir exoscale-capi-image && cd exoscale-capi-image
$ mkdir output packer-cache
$ KUBERNETES_VERSION=1.34.11
$ KUBERNETES_SERIES=v1.34
$ tee kubernetes.json >/dev/null <<EOF
{
  "kubernetes_deb_version": "${KUBERNETES_VERSION}-1.1",
  "kubernetes_rpm_version": "${KUBERNETES_VERSION}",
  "kubernetes_semver": "v${KUBERNETES_VERSION}",
  "kubernetes_series": "${KUBERNETES_SERIES}"
}
EOF
```

`-1.1` is the Debian package revision published by the Kubernetes package
repository. It is not part of the Kubernetes version.

Validate and build the UEFI QCOW2 with the released image-builder container:

```bash
$ IMAGE_BUILDER_VERSION=v0.1.55
$ podman run --rm --pull=always \
    --device /dev/kvm \
    --env PACKER_VAR_FILES=/home/imagebuilder/kubernetes.json \
    --volume "$PWD/kubernetes.json:/home/imagebuilder/kubernetes.json:ro" \
    "registry.k8s.io/scl-image-builder/cluster-node-image-builder-amd64:${IMAGE_BUILDER_VERSION}" \
    validate-qemu-ubuntu-2404-efi

$ podman run --rm \
    --userns=keep-id:uid=1001,gid=1001 \
    --device /dev/kvm \
    --env PACKER_VAR_FILES=/home/imagebuilder/kubernetes.json \
    --volume "$PWD/kubernetes.json:/home/imagebuilder/kubernetes.json:ro" \
    --volume "$PWD/output:/home/imagebuilder/output" \
    --volume "$PWD/packer-cache:/home/imagebuilder/.cache/packer" \
    "registry.k8s.io/scl-image-builder/cluster-node-image-builder-amd64:${IMAGE_BUILDER_VERSION}" \
    build-qemu-ubuntu-2404-efi
```

`--userns=keep-id:uid=1001,gid=1001` lets the rootless container write the
mounted output directory. A successful build runs image-builder's goss suite
and creates a 20 GiB virtual disk at:

```bash
$ ARTIFACT="$PWD/output/ubuntu-2404-efi-kube-v${KUBERNETES_VERSION}/ubuntu-2404-efi-kube-v${KUBERNETES_VERSION}"
$ qemu-img check "$ARTIFACT"
```

## Register the Exoscale template

Registration downloads the QCOW2 from an HTTP(S) URL and verifies its MD5. The
following commands use a private SOS object and a temporary pre-signed URL.
Choose a globally unique lowercase bucket name. The template is private and
zone-local. Custom-template storage is billed until the template is deleted.

```bash
$ ZONE=ch-gva-2
$ BUCKET=replace-with-a-globally-unique-bucket
$ OBJECT="exoscale-capi-ubuntu-2404-k8s-v${KUBERNETES_VERSION}.qcow2"
$ TEMPLATE_NAME="exoscale-capi-ubuntu-2404-k8s-v${KUBERNETES_VERSION}"
$ CHECKSUM=$(md5sum "$ARTIFACT" | cut -d' ' -f1)

$ exo storage mb "sos://${BUCKET}" --zone "$ZONE" --acl private
$ AWS_MAX_ATTEMPTS=10 AWS_RETRY_MODE=adaptive \
    exo storage upload --acl private "$ARTIFACT" "sos://${BUCKET}/${OBJECT}"
$ URL=$(exo storage presign "sos://${BUCKET}/${OBJECT}" --expires 6h -Q)
$ exo compute instance-template register \
    "$TEMPLATE_NAME" "$URL" "$CHECKSUM" \
    --zone "$ZONE" \
    --boot-mode uefi \
    --username ubuntu \
    --disable-password \
    --version "v${KUBERNETES_VERSION}" \
    --output-format json > template.json
$ TEMPLATE_ID=$(jq -er '.id' template.json)
$ rm -f template.json
$ unset URL
```

The register command waits for the import to finish before returning. The
pre-signed URL and temporary object are no longer needed after it succeeds:

```bash
$ exo storage rb "sos://${BUCKET}" --recursive --force
```

## Create the cluster

In `kustomization.yaml`:

1. Replace `REPLACE_WITH_TEMPLATE_UUID` with the value stored in `$TEMPLATE_ID`.
2. Change `v1.34.11` if the image contains another Kubernetes version.

This sample creates one control-plane Node and one worker Node. Both use the
private template.

From the Exoscale CAPI repository root, [install the released provider] as
shown by the base sample. To develop the provider from this checkout instead,
follow its [development workflow] and keep the manager running in another
terminal.

The sample creates two billable VMs and one billable private template in
`ch-gva-2`.

```bash
$> export EXOSCALE_API_KEY=<api-key>
$> export EXOSCALE_API_SECRET=<api-secret>
$> kubectl create secret generic exoscale --from-literal=apikey=$EXOSCALE_API_KEY --from-literal=apisecret=$EXOSCALE_API_SECRET
$> kubectl apply -k config/samples/kubeadm/cluster-custom-image/
```

Use the standard sample's [wait and smoke-test steps] to verify the cluster,
then follow its [scaling steps].

## Deploy the Exoscale CSI driver

Create a separate API key with the permissions listed in the [CSI driver
prerequisites], then store it in the workload cluster:

```bash
$> export EXOSCALE_CSI_API_KEY=<api-key>
$> export EXOSCALE_CSI_API_SECRET=<api-secret>
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    create secret generic exoscale-credentials \
    --namespace kube-system \
    --from-literal=EXOSCALE_API_KEY="$EXOSCALE_CSI_API_KEY" \
    --from-literal=EXOSCALE_API_SECRET="$EXOSCALE_CSI_API_SECRET"
```

Install the driver and its snapshot CRDs:

```bash
$> export CSI_VERSION=0.34.4
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    apply -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/v${CSI_VERSION}/deployment/${CSI_VERSION}/crds.yaml"
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    wait --for=condition=Established --timeout=60s \
    crd/volumesnapshotclasses.snapshot.storage.k8s.io \
    crd/volumesnapshotcontents.snapshot.storage.k8s.io \
    crd/volumesnapshots.snapshot.storage.k8s.io
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    apply -k "github.com/exoscale/exoscale-csi-driver/deployment/${CSI_VERSION}?ref=v${CSI_VERSION}"
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" --namespace kube-system \
    rollout status deployment/exoscale-csi-controller --timeout=5m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" --namespace kube-system \
    rollout status daemonset/exoscale-csi-node --timeout=5m
```

## Provision a volume

Deploy the CSI driver's PVC example and wait for its pod:

```bash
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" apply \
    -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/v${CSI_VERSION}/doc/examples/namespace.yaml" \
    -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/v${CSI_VERSION}/doc/examples/pvc.yaml" \
    -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/v${CSI_VERSION}/doc/examples/deployment.yaml"
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    wait deployment/my-awesome-deployment -n awesome \
    --for=condition=Available --timeout=5m
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get pods,pvc -n awesome
```

Delete the example and wait for the CSI driver to remove its backing volume
before deleting the cluster:

```bash
$> PV_NAME=$(kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    get pvc/my-sbs-pvc -n awesome -o jsonpath='{.spec.volumeName}')
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" delete \
    -f "https://raw.githubusercontent.com/exoscale/exoscale-csi-driver/v${CSI_VERSION}/doc/examples/namespace.yaml"
$> kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" \
    wait "pv/$PV_NAME" --for=delete --timeout=5m
```

Delete the Cluster before deleting its private template so the provider can
remove the Exoscale resources first:

```console
$ kubectl delete cluster/my-cluster
$ rm -f "$WORKLOAD_KUBECONFIG"
$ exo compute instance-template delete <template-id> --zone ch-gva-2
```

[Kubernetes image-builder]: https://github.com/kubernetes-sigs/image-builder
[Exoscale CSI driver]: https://github.com/exoscale/exoscale-csi-driver
[CSI driver prerequisites]: https://github.com/exoscale/exoscale-csi-driver#prerequisite
[install the released provider]: ../cluster/README.md#deploy-cluster-api-components
[kubeadm cluster sample]: ../cluster/README.md
[development workflow]: ../cluster/README.md#development-from-source
[scaling steps]: ../cluster/README.md#scale-the-cluster
[wait and smoke-test steps]: ../cluster/README.md#wait-for-the-workload-cluster
