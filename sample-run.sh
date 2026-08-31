#!/usr/bin/env bash

# The script creates a local Kind management cluster, runs Exoscale CAPI in the
# background, creates a real Exoscale workload cluster, and waits for
# confirmation before cleanup. The Exoscale VM, Elastic IP, and security
# groups are billable until cleanup finishes.

set -Eeuo pipefail
umask 077

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
KIND_CLUSTER=${KIND_CLUSTER:-capi-sample}
MANAGEMENT_KUBECONFIG=${MANAGEMENT_KUBECONFIG:-/tmp/${KIND_CLUSTER}-management.kubeconfig}
WORKLOAD_KUBECONFIG=${WORKLOAD_KUBECONFIG:-/tmp/${KIND_CLUSTER}-workload.kubeconfig}
CAPI_LOG=${CAPI_LOG:-/tmp/${KIND_CLUSTER}-controller.log}
CAPI_PID_FILE=${CAPI_PID_FILE:-/tmp/${KIND_CLUSTER}-controller.pid}
RUNNER_PID_FILE=${RUNNER_PID_FILE:-/tmp/${KIND_CLUSTER}-runner.pid}
EXOSCALE_CONFIG=${EXOSCALE_CONFIG:-${HOME}/.config/exoscale/exoscale.toml}
SAMPLE=${SAMPLE:-traditional}

case "$SAMPLE" in
traditional) SAMPLE_DIR=config/samples/kubeadm/cluster ;;
custom-image) SAMPLE_DIR=config/samples/kubeadm/cluster-custom-image ;;
*)
	printf 'Unknown sample %q; expected traditional or custom-image.\n' "$SAMPLE" >&2
	exit 1
	;;
esac

CAPI_PID=
KIND_CREATED=false

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

stop_capi() {
	if [[ -n "$CAPI_PID" ]] && kill -0 "$CAPI_PID" 2>/dev/null; then
		# setsid gives make/go/controller one process group, so stop all of it.
		kill -- "-$CAPI_PID" 2>/dev/null || true
		wait "$CAPI_PID" 2>/dev/null || true
	fi
	rm -f -- "$CAPI_PID_FILE"
}

cleanup() {
	status=$?
	trap - EXIT
	set +e

	if [[ "$KIND_CREATED" == true ]]; then
		export KUBECONFIG=$MANAGEMENT_KUBECONFIG

		# Keep CAPI alive until CAPI finalizers remove the real cloud resources.
		cluster_resource=
		# A missing Cluster CRD proves the script failed before workload creation.
		if ! cluster_crd=$(kubectl get customresourcedefinition clusters.cluster.x-k8s.io --ignore-not-found -o name); then
			printf '\nCould not verify workload-cluster state. Kind and CAPI were left running.\n' >&2
			printf 'Inspect CAPI with: tail -f %s\n' "$CAPI_LOG" >&2
			printf 'Use management kubeconfig: export KUBECONFIG=%s\n' "$MANAGEMENT_KUBECONFIG" >&2
			exit 1
		fi
		if [[ -n "$cluster_crd" ]] && ! cluster_resource=$(kubectl get cluster my-cluster --ignore-not-found -o name); then
			printf '\nCould not verify workload-cluster state. Kind and CAPI were left running.\n' >&2
			printf 'Inspect CAPI with: tail -f %s\n' "$CAPI_LOG" >&2
			printf 'Use management kubeconfig: export KUBECONFIG=%s\n' "$MANAGEMENT_KUBECONFIG" >&2
			exit 1
		fi
		if [[ -n "$cluster_resource" ]]; then
			printf '\nDeleting workload cluster and Exoscale resources...\n'
			if ! kubectl delete cluster my-cluster --wait --timeout=10m; then
				printf '\nCleanup did not finish. Kind and CAPI were left running.\n' >&2
				printf 'Inspect CAPI with: tail -f %s\n' "$CAPI_LOG" >&2
				printf 'Use management kubeconfig: export KUBECONFIG=%s\n' "$MANAGEMENT_KUBECONFIG" >&2
				exit 1
			fi
		fi

		kubectl delete secret exoscale --ignore-not-found >/dev/null 2>&1 || true
		stop_capi
		if ! kind delete cluster --name "$KIND_CLUSTER"; then
			printf 'Cloud cleanup finished, but Kind cleanup failed. Local state was retained.\n' >&2
			exit 1
		fi
		rm -f -- "$MANAGEMENT_KUBECONFIG" "$WORKLOAD_KUBECONFIG" "$RUNNER_PID_FILE"
		printf 'Cleanup complete. Controller log retained at %s\n' "$CAPI_LOG"
	else
		stop_capi
		rm -f -- "$RUNNER_PID_FILE"
	fi

	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT TERM

for tool in docker kind kubectl make go curl setsid; do
	require "$tool"
done

if ! docker info >/dev/null 2>&1; then
	printf 'Docker is installed, but its daemon is unavailable.\n' >&2
	exit 1
fi

if [[ ! -f "$EXOSCALE_CONFIG" ]]; then
	printf 'Exoscale CLI configuration not found: %s\n' "$EXOSCALE_CONFIG" >&2
	exit 1
fi

# Refuse to reuse anything so cleanup cannot delete an unrelated local cluster.
if kind get kubeconfig --name "$KIND_CLUSTER" >/dev/null 2>&1; then
	printf 'Kind cluster already exists: %s\n' "$KIND_CLUSTER" >&2
	printf 'A previous sample may still be running; finish its cleanup before retrying.\n' >&2
	exit 1
fi
if [[ -e "$MANAGEMENT_KUBECONFIG" || -e "$WORKLOAD_KUBECONFIG" ]]; then
	printf 'A sample kubeconfig already exists under /tmp; remove it before retrying.\n' >&2
	exit 1
fi

cd -- "$ROOT"
printf '%s\n' "$$" >"$RUNNER_PID_FILE"
CAPI_VERSION=$(go list -m -f '{{.Version}}' sigs.k8s.io/cluster-api)

# Download repository-pinned tools. clusterctl installs CAPI into Kind; yq
# reads the existing Exoscale CLI configuration without printing credentials.
make clusterctl yq

printf '\nCreating Kind management cluster %s...\n' "$KIND_CLUSTER"
kind create cluster --name "$KIND_CLUSTER" --kubeconfig "$MANAGEMENT_KUBECONFIG"
KIND_CREATED=true
export KUBECONFIG=$MANAGEMENT_KUBECONFIG

printf '\nInstalling CAPI core and kubeadm providers...\n'
./bin/clusterctl init \
	--core "cluster-api:$CAPI_VERSION" \
	--bootstrap "kubeadm:$CAPI_VERSION" \
	--control-plane "kubeadm:$CAPI_VERSION" \
	--infrastructure -

printf '\nInstalling CAPI CRDs...\n'
make install

# Read the default Exoscale account. Values stay in shell variables and are
# passed to kubectl through file descriptors, not command-line arguments.
export EXOSCALE_ACCOUNT
EXOSCALE_ACCOUNT=$(./bin/yq -r '.defaultaccount' "$EXOSCALE_CONFIG")
EXOSCALE_API_KEY=$(./bin/yq -r '.accounts[] | select(.name == env(EXOSCALE_ACCOUNT)) | .key' "$EXOSCALE_CONFIG")
EXOSCALE_API_SECRET=$(./bin/yq -r '.accounts[] | select(.name == env(EXOSCALE_ACCOUNT)) | .secret' "$EXOSCALE_CONFIG")

if [[ -z "$EXOSCALE_ACCOUNT" || "$EXOSCALE_ACCOUNT" == null ||
	-z "$EXOSCALE_API_KEY" || "$EXOSCALE_API_KEY" == null ||
	-z "$EXOSCALE_API_SECRET" || "$EXOSCALE_API_SECRET" == null ]]; then
	printf 'Could not load the default Exoscale account credentials.\n' >&2
	exit 1
fi

kubectl create secret generic exoscale \
	--from-file=apikey=<(printf '%s' "$EXOSCALE_API_KEY") \
	--from-file=apisecret=<(printf '%s' "$EXOSCALE_API_SECRET") \
	--dry-run=client -o yaml | kubectl apply -f -
unset EXOSCALE_API_KEY EXOSCALE_API_SECRET

# Run CAPI in a separate process group so this terminal remains available and
# cleanup can reliably stop make, go run, and the controller together.
printf '\nStarting CAPI; log: %s\n' "$CAPI_LOG"
: >"$CAPI_LOG"
setsid make run >"$CAPI_LOG" 2>&1 &
CAPI_PID=$!
printf '%s\n' "$CAPI_PID" >"$CAPI_PID_FILE"

ready=false
for ((attempt = 1; attempt <= 180; attempt++)); do
	if ! kill -0 "$CAPI_PID" 2>/dev/null; then
		printf 'CAPI exited before becoming ready. Inspect %s\n' "$CAPI_LOG" >&2
		exit 1
	fi
	if curl --fail --silent http://127.0.0.1:8081/readyz >/dev/null; then
		ready=true
		break
	fi
	sleep 1
done
if [[ "$ready" != true ]]; then
	printf 'CAPI did not become ready within 3 minutes. Inspect %s\n' "$CAPI_LOG" >&2
	exit 1
fi

# clusterctl can return before every CAPI admission webhook accepts traffic.
# Retry a server-side dry run so a startup race cannot leave half of the real
# sample persisted. A genuine validation error remains visible after 3 minutes.
printf '\nWaiting for CAPI admission webhooks...\n'
webhooks_ready=false
last_webhook_error=
for ((attempt = 1; attempt <= 90; attempt++)); do
	if last_webhook_error=$(kubectl apply --server-side --dry-run=server -k "$SAMPLE_DIR" 2>&1); then
		webhooks_ready=true
		break
	fi
	if ! kill -0 "$CAPI_PID" 2>/dev/null; then
		printf 'CAPI exited while waiting for admission webhooks. Inspect %s\n' "$CAPI_LOG" >&2
		exit 1
	fi
	sleep 2
done
if [[ "$webhooks_ready" != true ]]; then
	printf 'CAPI admission webhooks did not accept the sample within 3 minutes:\n%s\n' "$last_webhook_error" >&2
	exit 1
fi

# The sample creates one real control-plane VM in ch-gva-2. kubeadm initializes
# it and Flannel supplies the Pod network.
printf '\nCreating the Exoscale workload cluster...\n'
kubectl apply -k "$SAMPLE_DIR"

kubectl wait exoscalecluster/my-cluster --for=condition=Ready --timeout=5m
kubectl wait kubeadmcontrolplane/my-control-plane --for=condition=Available --timeout=15m

./bin/clusterctl get kubeconfig my-cluster >"$WORKLOAD_KUBECONFIG"
node_found=false
for ((attempt = 1; attempt <= 300; attempt++)); do
	if [[ -n "$(kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get nodes -o name 2>/dev/null)" ]]; then
		node_found=true
		break
	fi
	sleep 2
done
if [[ "$node_found" != true ]]; then
	printf 'No workload Node registered within 10 minutes.\n' >&2
	exit 1
fi
kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" wait node --all --for=condition=Ready --timeout=10m

kubectl wait cluster/my-cluster --for=condition=ControlPlaneInitialized --timeout=5m
kubectl wait cluster/my-cluster --for=condition=RemoteConnectionProbe --timeout=5m
kubectl wait machine \
	--selector=cluster.x-k8s.io/cluster-name=my-cluster,cluster.x-k8s.io/control-plane \
	--for=condition=Ready --timeout=5m
kubectl wait machine \
	--selector=cluster.x-k8s.io/cluster-name=my-cluster,cluster.x-k8s.io/control-plane \
	--for=condition=Available --timeout=5m

printf '\nManagement-cluster resources:\n'
kubectl get cluster,kubeadmcontrolplane,exoscalecluster,exoscalemachinetemplate,machine,exoscalemachine
printf '\nWorkload-cluster node:\n'
kubectl --kubeconfig="$WORKLOAD_KUBECONFIG" get nodes -o wide

printf '\nThe sample is ready.\n'
printf 'Management cluster: kubectl --kubeconfig=%q get cluster,kubeadmcontrolplane,exoscalecluster,exoscalemachinetemplate,machine,exoscalemachine\n' "$MANAGEMENT_KUBECONFIG"
printf 'Workload cluster:   kubectl --kubeconfig=%q get pods -A\n' "$WORKLOAD_KUBECONFIG"
printf 'Controller logs:    tail -f %q\n' "$CAPI_LOG"
printf '\nPress Enter to delete the workload cluster, stop CAPI, and remove Kind.\n'
read -r
