#!/usr/bin/env bash

set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir "$TMP/bin"

cat >"$TMP/bin/kind" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "get clusters") printf '%s\n' sample-test ;;
  "delete cluster --name sample-test") printf '%s\n' kind-delete >>"$CALLS" ;;
  *) printf 'unexpected kind call: %s\n' "$*" >&2; exit 97 ;;
esac
EOF

cat >"$TMP/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "get customresourcedefinition clusters.cluster.x-k8s.io --ignore-not-found -o name")
    case "$SCENARIO" in
      missing-crd) ;;
      api-error) exit 1 ;;
      workload|delete-error) printf '%s\n' customresourcedefinition.apiextensions.k8s.io/clusters.cluster.x-k8s.io ;;
    esac
    ;;
  "get cluster my-cluster --namespace default --ignore-not-found -o name")
    printf '%s\n' cluster.cluster.x-k8s.io/my-cluster
    ;;
  "wait deployment/cluster-api-provider-exoscale-controller-manager --namespace cluster-api-provider-exoscale-system --for=condition=Available --timeout=10s")
    exit 1
    ;;
  "delete cluster my-cluster --namespace default --wait --timeout=10m")
    printf '%s\n' cluster-delete >>"$CALLS"
	[[ "$SCENARIO" != delete-error ]]
    ;;
  "delete secret exoscale --namespace default --ignore-not-found") ;;
  *) printf 'unexpected kubectl call: %s\n' "$*" >&2; exit 97 ;;
esac
EOF

cat >"$TMP/bin/make" <<'EOF'
#!/usr/bin/env bash
printf 'make %s\n' "$*" >>"$CALLS"
EOF

chmod +x "$TMP/bin/kind" "$TMP/bin/kubectl" "$TMP/bin/make"

run_case() {
  scenario=$1
  expected_status=$2
  expected_calls=$3
  calls="$TMP/$scenario.calls"
  kubeconfig="$TMP/$scenario.kubeconfig"
  : >"$calls"
  : >"$kubeconfig"

  set +e
  PATH="$TMP/bin:$PATH" CALLS="$calls" SCENARIO="$scenario" \
    KIND_CLUSTER=sample-test MANAGEMENT_KUBECONFIG="$kubeconfig" \
    WORKLOAD_KUBECONFIG="$TMP/$scenario-workload.kubeconfig" \
    RUNNER_PID_FILE="$TMP/$scenario-runner.pid" \
    bash "$ROOT/sample-clean.sh" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" == "$expected_status" ]] || {
    printf '%s: expected exit %s, got %s\n' "$scenario" "$expected_status" "$status" >&2
    return 1
  }
  actual_calls=$(<"$calls")
  [[ "$actual_calls" == "$expected_calls" ]] || {
    printf '%s: unexpected calls:\n%s\n' "$scenario" "$actual_calls" >&2
    return 1
  }
}

run_case missing-crd 0 "kind-delete"
run_case api-error 1 ""
run_case workload 0 $'make run LOCAL_KIND_CLUSTER=sample-test LOCAL_IMG=localhost/cluster-api-provider-exoscale:sample\ncluster-delete\nkind-delete'
run_case delete-error 1 $'make run LOCAL_KIND_CLUSTER=sample-test LOCAL_IMG=localhost/cluster-api-provider-exoscale:sample\ncluster-delete'

printf 'sample cleanup checks passed\n'
