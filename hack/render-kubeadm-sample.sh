#!/usr/bin/env bash

set -Eeuo pipefail

if [[ "$#" -ne 1 ]]; then
	printf 'Usage: %s <sample-directory>\n' "$0" >&2
	exit 1
fi

KUSTOMIZE=${KUSTOMIZE:-bin/kustomize}
YQ=${YQ:-bin/yq}
CUSTOM_IMAGE_TEMPLATE=${CUSTOM_IMAGE_TEMPLATE:-}
CUSTOM_IMAGE_KUBERNETES_VERSION=${CUSTOM_IMAGE_KUBERNETES_VERSION:-}

if [[ -z "$CUSTOM_IMAGE_TEMPLATE" && -z "$CUSTOM_IMAGE_KUBERNETES_VERSION" ]]; then
	exec "$KUSTOMIZE" build "$1"
fi
if [[ -z "$CUSTOM_IMAGE_TEMPLATE" || ! "$CUSTOM_IMAGE_KUBERNETES_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	printf 'CUSTOM_IMAGE_TEMPLATE and an exact v-prefixed CUSTOM_IMAGE_KUBERNETES_VERSION are required together.\n' >&2
	exit 1
fi

export CUSTOM_IMAGE_TEMPLATE CUSTOM_IMAGE_KUBERNETES_VERSION
"$KUSTOMIZE" build "$1" | "$YQ" eval \
	'(select(.kind == "ExoscaleMachineTemplate") | .spec.template.spec.template) = strenv(CUSTOM_IMAGE_TEMPLATE) |
	 (select(.kind == "KubeadmControlPlane") | .spec.version) = strenv(CUSTOM_IMAGE_KUBERNETES_VERSION)' \
	-
