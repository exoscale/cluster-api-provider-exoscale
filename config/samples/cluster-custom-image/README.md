# Pre-built Kubernetes image

This overlay runs the kubeadm cluster sample with Kubernetes pre-installed in a
private Exoscale template. It uses the generic Ubuntu 24.04 UEFI QEMU target
from [Kubernetes image-builder]. No Exoscale-specific image-builder target is
required.

The commands below were tested with image-builder `v0.1.55`, Ubuntu `24.04.4`,
Kubernetes `v1.36.4`, and containerd `2.3.2` on amd64.

## Image requirements

The image must include cloud-init with the Exoscale datasource, containerd,
kubeadm, kubelet, and kubectl. The Kubernetes version baked into the image must
match `KubeadmControlPlane.spec.version` in `kustomization.yaml`.

The provider ID cannot be baked into the image because it contains the new VM's
UUID. This overlay therefore removes package installation from cloud-init but
keeps the per-instance provider-ID command.

## Create the image

Requirements: Podman, KVM exposed as `/dev/kvm`, at least 15 GiB of free disk,
and `qemu-img` for the final check.

Create a working directory and an image-builder version override:

```bash
$ mkdir exoscale-capi-image && cd exoscale-capi-image
$ mkdir output packer-cache
$ KUBERNETES_VERSION=1.36.4
$ KUBERNETES_SERIES=v1.36
$ tee kubernetes.json >/dev/null <<EOF
{
  "kubernetes_deb_version": "${KUBERNETES_VERSION}-1.1",
  "kubernetes_rpm_version": "${KUBERNETES_VERSION}",
  "kubernetes_semver": "v${KUBERNETES_VERSION}",
  "kubernetes_series": "${KUBERNETES_SERIES}"
}
EOF
```

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
    --output-format json
$ unset URL
```

Record the returned template `id`, then remove the temporary object after
registration finishes:

```bash
$ exo storage rb "sos://${BUCKET}" --recursive --force
```

## Create the cluster

In `kustomization.yaml`:

1. Replace `REPLACE_WITH_TEMPLATE_UUID` with the registered template ID.
2. Change `v1.36.4` if the image contains another Kubernetes version.

From the Exoscale CAPI repository root, run:

```console
$ SAMPLE=custom-image ./sample-run.sh
```

The script waits for the kubeadm control plane and its Node to become Ready.
Press Enter when prompted to delete the workload cluster and its Exoscale
resources. The private template remains until it is deleted explicitly.

```console
$ exo compute instance-template delete <template-id> --zone ch-gva-2
```

[Kubernetes image-builder]: https://github.com/kubernetes-sigs/image-builder
