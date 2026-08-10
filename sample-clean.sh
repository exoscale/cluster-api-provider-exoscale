#!/usr/bin/env bash

# Emergency cleanup for sample-run.sh.
#
# Delete the CAPI Cluster first while Exoscale CAPI is running. CAPI owner references
# and Exoscale CAPI finalizers then remove the Exoscale VM, Elastic IP, and security
# groups. Kind is deleted only after that operation succeeds.

set -Eeuo pipefail
umask 077

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
KIND_CLUSTER=${KIND_CLUSTER:-capi-sample}
MANAGEMENT_KUBECONFIG=${MANAGEMENT_KUBECONFIG:-/tmp/${KIND_CLUSTER}-management.kubeconfig}
WORKLOAD_KUBECONFIG=${WORKLOAD_KUBECONFIG:-/tmp/${KIND_CLUSTER}-workload.kubeconfig}
RUNNER_PID_FILE=${RUNNER_PID_FILE:-/tmp/${KIND_CLUSTER}-runner.pid}
LOCAL_IMG=${LOCAL_IMG:-localhost/cluster-api-provider-exoscale:sample}
CAPX_NAMESPACE=cluster-api-provider-exoscale-system
CAPX_DEPLOYMENT=cluster-api-provider-exoscale-controller-manager

timestamp_output() {
	while IFS= read -r line || [[ -n "$line" ]]; do
		printf '[%(%Y-%m-%d %H:%M:%S)T] %s\n' -1 "$line"
	done
}

exec > >(timestamp_output)
exec 2> >(timestamp_output >&2)

require() {
	command -v "$1" >/dev/null 2>&1 || {
		printf 'Missing required command: %s\n' "$1" >&2
		exit 1
	}
}

print_capx_logs() {
	printf 'Logs: KUBECONFIG=%q kubectl logs --namespace %q deployment/%q --container manager\n' \
		"$MANAGEMENT_KUBECONFIG" "$CAPX_NAMESPACE" "$CAPX_DEPLOYMENT" >&2
}

ensure_capx() {
	if kubectl wait deployment/"$CAPX_DEPLOYMENT" --namespace "$CAPX_NAMESPACE" \
		--for=condition=Available --timeout=10s >/dev/null 2>&1; then
		printf 'Using the running in-cluster CAPX deployment.\n'
		return
	fi

	printf 'Rebuilding and deploying CAPX for finalizer cleanup.\n'
	make run LOCAL_KIND_CLUSTER="$KIND_CLUSTER" LOCAL_IMG="$LOCAL_IMG"
}

signal_runner() {
	[[ -f "$RUNNER_PID_FILE" ]] || return 0
	read -r runner_pid <"$RUNNER_PID_FILE"
	if [[ "$runner_pid" =~ ^[0-9]+$ ]] && kill -0 "$runner_pid" 2>/dev/null; then
		args=$(ps -o args= -p "$runner_pid" 2>/dev/null || true)
		if [[ "$args" == *"sample-run.sh"* ]]; then
			kill -TERM "$runner_pid" 2>/dev/null || true
			for ((attempt = 1; attempt <= 100; attempt++)); do
				kill -0 "$runner_pid" 2>/dev/null || break
				sleep 0.1
			done
			if kill -0 "$runner_pid" 2>/dev/null; then
				printf 'The sample runner did not stop; local state was retained.\n' >&2
				return 1
			fi
		fi
	fi
	rm -f -- "$RUNNER_PID_FILE"
}

for tool in kind kubectl make go ps; do
	require "$tool"
done

cd -- "$ROOT"
signal_runner

if ! kind_clusters=$(kind get clusters); then
	printf 'Could not determine whether Kind cluster %s exists; no cleanup was attempted.\n' "$KIND_CLUSTER" >&2
	exit 1
fi
kind_cluster_exists=false
while IFS= read -r cluster; do
	if [[ "$cluster" == "$KIND_CLUSTER" ]]; then
		kind_cluster_exists=true
		break
	fi
done <<<"$kind_clusters"

if [[ "$kind_cluster_exists" != true ]]; then
	printf 'Kind cluster %s is already absent.\n' "$KIND_CLUSTER"
	rm -f -- "$MANAGEMENT_KUBECONFIG" "$WORKLOAD_KUBECONFIG"
	printf 'Removed stale local sample state. Cloud cleanup cannot be verified without the management cluster.\n'
	exit 1
fi

# Recreate the management kubeconfig from Kind if the original file was lost.
if [[ ! -f "$MANAGEMENT_KUBECONFIG" ]]; then
	kind get kubeconfig --name "$KIND_CLUSTER" >"$MANAGEMENT_KUBECONFIG"
fi
export KUBECONFIG=$MANAGEMENT_KUBECONFIG

cluster_resource=
if ! cluster_crd=$(kubectl get customresourcedefinition clusters.cluster.x-k8s.io --ignore-not-found -o name); then
	printf 'Could not verify workload-cluster state. Kind and CAPI were left running.\n' >&2
	exit 1
fi
if [[ -n "$cluster_crd" ]] && ! cluster_resource=$(kubectl get cluster my-cluster --namespace default --ignore-not-found -o name); then
	printf 'Could not verify workload-cluster state. Kind and CAPI were left running.\n' >&2
	exit 1
fi
if [[ -n "$cluster_resource" ]]; then
	ensure_capx

	printf 'Deleting Cluster/my-cluster and waiting for cloud finalizers...\n'
	if ! kubectl delete cluster my-cluster --namespace default --wait --timeout=10m; then
		printf '\nCleanup failed. Kind and CAPI were deliberately left running.\n' >&2
		printf 'Inspect: KUBECONFIG=%s kubectl get cluster,kubeadmcontrolplane,exoscalecluster,exoscalemachinetemplate,machine,exoscalemachine\n' "$MANAGEMENT_KUBECONFIG" >&2
		print_capx_logs
		exit 1
	fi
else
	printf 'Cluster/my-cluster is already absent; no CAPI workload remains.\n'
fi

kubectl delete secret exoscale --namespace default --ignore-not-found >/dev/null 2>&1 || true

kind delete cluster --name "$KIND_CLUSTER"
rm -f -- "$MANAGEMENT_KUBECONFIG" "$WORKLOAD_KUBECONFIG"

printf 'Emergency cleanup complete.\n'
