#!/usr/bin/env bash
# Dump the state of the management (Kind) cluster after an e2e run so CI failures
# that only reproduce in the pipeline (control plane never initializing, k0smotron
# controllers crashlooping, image pull errors, ...) can be diagnosed from the logs.
#
# Best-effort: every command is allowed to fail so one missing resource never
# aborts the rest of the dump.
set -u

KUBECTL="${KUBECTL:-kubectl}"
OUT_DIR="${1:-e2e-diagnostics}"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# Append one command and its output to a file; multiple calls with the same file
# accumulate rather than overwrite.
run() {
	local file="$1"
	shift
	{
		echo "+ $*"
		"$@" 2>&1 || echo "(command failed: exit $?)"
		echo
	} >>"$OUT_DIR/$file"
}

echo "Dumping e2e diagnostics to $OUT_DIR"

# Cluster-wide overview.
run overview.txt "$KUBECTL" get nodes -o wide
run overview.txt "$KUBECTL" get pods -A -o wide
run overview.txt "$KUBECTL" get events -A --sort-by=.lastTimestamp

# CAPI / bootstrap / control-plane / infra resources across all namespaces.
for kind in \
	clusters.cluster.x-k8s.io \
	machinedeployments.cluster.x-k8s.io \
	machinesets.cluster.x-k8s.io \
	machines.cluster.x-k8s.io \
	k0scontrolplanes.controlplane.cluster.x-k8s.io \
	k0sworkerconfigs.bootstrap.cluster.x-k8s.io \
	k0sworkerconfigtemplates.bootstrap.cluster.x-k8s.io \
	k0scontrollerconfigs.bootstrap.cluster.x-k8s.io \
	exoscaleclusters.infrastructure.cluster.x-k8s.io \
	exoscalemachines.infrastructure.cluster.x-k8s.io \
	exoscalemachinetemplates.infrastructure.cluster.x-k8s.io; do
	run "capi-${kind%%.*}.yaml" "$KUBECTL" get "$kind" -A -o yaml
	run "capi-${kind%%.*}.describe.txt" "$KUBECTL" describe "$kind" -A
done

# Controller logs (current + previous container in case of restarts).
dump_logs() {
	local ns="$1" selector="$2"
	run "logs-${ns}.txt" "$KUBECTL" -n "$ns" logs -l "$selector" --all-containers --tail=-1 --prefix
	run "logs-${ns}.prev.txt" "$KUBECTL" -n "$ns" logs -l "$selector" --all-containers --tail=-1 --prefix --previous
	run "logs-${ns}.txt" "$KUBECTL" -n "$ns" get pods -o wide
}

dump_logs cluster-api-provider-exoscale-system control-plane=controller-manager
dump_logs k0smotron control-plane=controller-manager
dump_logs capi-system cluster.x-k8s.io/provider=cluster-api

# Reachability of the k0s API. If a control plane is stuck "not initialized",
# this tells apart "k0s is down on the node" (neither address answers) from "the
# Elastic IP is not routing to the instance" (the instance's own IP answers but
# the EIP does not).
probe_kube_api() {
	local ns="$1" cluster="$2" eip ip addr
	# EIP the ExoscaleCluster advertises as the control-plane endpoint.
	eip=$("$KUBECTL" -n "$ns" get exoscalecluster "$cluster" -o jsonpath='{.status.controlPlaneEndpoint.host}' 2>/dev/null)
	# Primary public IP of a control-plane ExoscaleMachine for that cluster.
	ip=$("$KUBECTL" -n "$ns" get exoscalemachine \
		-l "cluster.x-k8s.io/cluster-name=${cluster},cluster.x-k8s.io/control-plane=true" \
		-o jsonpath='{.items[0].status.addresses[0].address}' 2>/dev/null)

	{
		echo "cluster: ${ns}/${cluster}"
		echo "advertised control-plane endpoint (Elastic IP): ${eip:-<none>}"
		echo "control-plane instance primary IP:              ${ip:-<none>}"
		for addr in "$eip" "$ip"; do
			[ -n "$addr" ] || continue
			echo "+ curl -sk --max-time 5 https://${addr}:6443/healthz"
			curl -sk --max-time 5 -o /dev/null -w '  http_code=%{http_code} time=%{time_total}s\n' \
				"https://${addr}:6443/healthz" 2>&1 || echo "  (unreachable: exit $?)"
		done
		echo
	} >>"$OUT_DIR/kube-api-reachability.txt"
}

"$KUBECTL" get exoscalecluster -A \
	-o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null \
	| while read -r ns name; do
		[ -n "$name" ] && probe_kube_api "$ns" "$name"
	done

echo "Done."
