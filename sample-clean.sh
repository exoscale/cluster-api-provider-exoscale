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
CAPI_LOG=${CAPI_LOG:-/tmp/${KIND_CLUSTER}-controller.log}
CAPI_PID_FILE=${CAPI_PID_FILE:-/tmp/${KIND_CLUSTER}-controller.pid}
RUNNER_PID_FILE=${RUNNER_PID_FILE:-/tmp/${KIND_CLUSTER}-runner.pid}

CAPI_PID=
STARTED_CAPI=false

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

# Only trust a recorded PID when it is still a process-group leader running
# this sample's `make run`. This avoids killing an unrelated reused PID.
load_capi_pid() {
	[[ -f "$CAPI_PID_FILE" ]] || return 1
	read -r candidate <"$CAPI_PID_FILE"
	[[ "$candidate" =~ ^[0-9]+$ ]] || return 1
	kill -0 "$candidate" 2>/dev/null || return 1

	pgid=$(ps -o pgid= -p "$candidate")
	pgid=${pgid//[[:space:]]/}
	args=$(ps -o args= -p "$candidate")
	[[ "$pgid" == "$candidate" && "$args" == *"make run"* ]] || return 1

	CAPI_PID=$candidate
}

stop_capi() {
	if [[ -n "$CAPI_PID" ]] && kill -0 "$CAPI_PID" 2>/dev/null; then
		kill -- "-$CAPI_PID" 2>/dev/null || true
		wait "$CAPI_PID" 2>/dev/null || true
	fi
	rm -f -- "$CAPI_PID_FILE"
}

wait_for_capi() {
	for ((attempt = 1; attempt <= 300; attempt++)); do
		if [[ -n "$CAPI_PID" ]] && ! kill -0 "$CAPI_PID" 2>/dev/null; then
			printf 'CAPI exited before becoming ready. Inspect %s\n' "$CAPI_LOG" >&2
			return 1
		fi
		if curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null; then
			return 0
		fi
		sleep 1
	done
	printf 'CAPI did not become ready within 5 minutes. Inspect %s\n' "$CAPI_LOG" >&2
	return 1
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

for tool in kind kubectl make go curl setsid ps; do
	require "$tool"
done

cd -- "$ROOT"

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
	load_capi_pid || true
	stop_capi
	signal_runner
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
if [[ -n "$cluster_crd" ]] && ! cluster_resource=$(kubectl get cluster my-cluster --ignore-not-found -o name); then
	printf 'Could not verify workload-cluster state. Kind and CAPI were left running.\n' >&2
	exit 1
fi
if [[ -n "$cluster_resource" ]]; then
	if load_capi_pid; then
		printf 'Using recorded CAPI process %s.\n' "$CAPI_PID"
	elif curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null 2>&1; then
		printf 'Using an already-running CAPI controller; it was not started by this helper.\n'
	else
		# Recovery path for a crashed/lost run script: restart CAPI long enough to
		# execute its deletion finalizers.
		printf 'Starting CAPI temporarily for finalizer cleanup; log: %s\n' "$CAPI_LOG"
		: >"$CAPI_LOG"
		setsid make run >"$CAPI_LOG" 2>&1 &
		CAPI_PID=$!
		STARTED_CAPI=true
		printf '%s\n' "$CAPI_PID" >"$CAPI_PID_FILE"
		wait_for_capi
	fi

	printf 'Deleting Cluster/my-cluster and waiting for cloud finalizers...\n'
	if ! kubectl delete cluster my-cluster --wait --timeout=10m; then
		printf '\nCleanup failed. Kind and CAPI were deliberately left running.\n' >&2
		printf 'Inspect: KUBECONFIG=%s kubectl get cluster,kubeadmcontrolplane,exoscalecluster,exoscalemachinetemplate,machine,exoscalemachine\n' "$MANAGEMENT_KUBECONFIG" >&2
		printf 'Logs:   tail -f %s\n' "$CAPI_LOG" >&2
		exit 1
	fi
else
	printf 'Cluster/my-cluster is already absent; no CAPI workload remains.\n'
	load_capi_pid || true
fi

kubectl delete secret exoscale --ignore-not-found >/dev/null 2>&1 || true

# Stop only a controller recorded by the sample or started by this helper.
if [[ -n "$CAPI_PID" || "$STARTED_CAPI" == true ]]; then
	stop_capi
fi

kind delete cluster --name "$KIND_CLUSTER"
signal_runner
rm -f -- "$MANAGEMENT_KUBECONFIG" "$WORKLOAD_KUBECONFIG" "$CAPI_PID_FILE"

printf 'Emergency cleanup complete. Controller log retained at %s\n' "$CAPI_LOG"
