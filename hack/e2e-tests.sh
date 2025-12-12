#!/usr/bin/env bash

# E2E test script for lbconfig-operator
# This script runs comprehensive end-to-end tests against a KIND cluster

set -e

OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE:-lbconfig-operator-system}"
CR_NAMESPACE="${CR_NAMESPACE:-lbconfig-operator-system}"
TIMEOUT="${TIMEOUT:-120s}"
VERBOSE="${VERBOSE:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

EXIT_ERROR=0

log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

check_resource() {
  local resource_type=$1
  local resource_name=$2
  local namespace=$3

  if kubectl get "$resource_type" "$resource_name" -n "$namespace" &>/dev/null; then
    log_info "$resource_type/$resource_name exists in namespace $namespace"
    return 0
  else
    log_error "$resource_type/$resource_name not found in namespace $namespace"
    return 1
  fi
}

wait_for_condition() {
  local resource_type=$1
  local resource_name=$2
  local condition=$3
  local namespace=$4
  local timeout=$5

  log_info "Waiting for $resource_type/$resource_name to meet condition: $condition"
  if kubectl wait --for="$condition" "$resource_type/$resource_name" -n "$namespace" --timeout="$timeout" 2>/dev/null; then
    log_info "Condition met: $condition"
    return 0
  else
    log_error "Timeout waiting for condition: $condition"
    return 1
  fi
}

deploy_instance() {
  log_info "=== Deploying lbconfig-operator Instance ==="

  log_info "Creating test resources..."
  kubectl apply -f examples/namespace.yaml
  kubectl apply --namespace lbconfig-operator-system -f examples/secret_v1_creds.yaml
  kubectl apply --namespace lbconfig-operator-system -f examples/lb_v1_externalloadbalancer-dummy.yaml

  log_info "Waiting for operator to reconcile..."
  sleep 10

  log_info "✅ Operator instance deployed"
}

test_operator_deployment() {
  log_info "=== Testing Operator Deployment ==="

  # Check if operator pod is running
  log_info "Checking operator pod status..."
  kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n "$OPERATOR_NAMESPACE" --timeout="$TIMEOUT"

  # Check operator logs for errors
  log_info "Checking operator logs for errors..."
  if kubectl logs -n "$OPERATOR_NAMESPACE" -l control-plane=controller-manager --tail=50 | grep -i "error" | grep -v "ignore"; then
    log_warn "Found error messages in operator logs (review above) ❎"
    EXIT_ERROR=1
  else
    log_info "No critical errors found in operator logs ✅"
  fi

  log_info "✅ Operator deployment validated"
}

test_basic_cr_lifecycle() {
  log_info "=== Testing Basic CR Lifecycle ==="

  local cr_name="externalloadbalancer-master-dummy-test"

  # Check CR exists
  check_resource "externalloadbalancer" "$cr_name" "$CR_NAMESPACE"

  # Wait for status to be populated
  log_info "Waiting for CR status to be populated..."
  sleep 5

  # Verify status fields
  log_info "Verifying status fields..."
  local numnodes=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.status.numnodes}')
  if [[ "$numnodes" -gt 0 ]]; then
    log_info "Status.numnodes = $numnodes ✅"
  else
    log_error "Status.numnodes not set or zero ❎"
    EXIT_ERROR=1
  fi

  # Check monitor name in status
  local monitor_name=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.status.monitor.name}')
  if [[ -n "$monitor_name" ]]; then
    log_info "Status.monitor.name = $monitor_name ✅"
  else
    log_error "Status.monitor.name not set ❎"
    EXIT_ERROR=1
  fi

  # Check pools in status
  local pool_count=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.status.pools}' | jq '. | length')
  if [[ "$pool_count" -gt 0 ]]; then
    log_info "Status.pools count = $pool_count ✅"
  else
    log_error "Status.pools not populated ❎"
    EXIT_ERROR=1
  fi

  log_info "✅ Basic CR lifecycle validated"
}

test_cr_update() {
  log_info "=== Testing CR Updates ==="

  local cr_name="externalloadbalancer-master-dummy-test"

  # Update VIP
  log_info "Updating VIP to 10.0.0.30..."
  kubectl patch elb "$cr_name" -n "$CR_NAMESPACE" --type=merge -p '{"spec":{"vip":"10.0.0.30"}}'

  sleep 5

  # Verify VIP updated
  local new_vip=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.spec.vip}')
  if [[ "$new_vip" == "10.0.0.30" ]]; then
    log_info "VIP updated successfully: $new_vip ✅"
  else
    log_error "VIP not updated. Current: $new_vip ❎"
    EXIT_ERROR=1
  fi

  # Add a new port
  log_info "Adding port 8443 to ports list..."
  kubectl patch elb "$cr_name" -n "$CR_NAMESPACE" --type=merge -p '{"spec":{"ports":[6443,8443]}}'

  sleep 5

  # Verify ports updated
  local ports=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.spec.ports}')
  if echo "$ports" | grep -q "8443"; then
    log_info "Ports updated successfully: $ports ✅"
  else
    log_error "Port 8443 not found in ports list ❎"
    EXIT_ERROR=1
  fi

  # Check that pool count increased
  local pool_count=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.status.pools}' | jq '. | length')
  if [[ "$pool_count" -ge 2 ]]; then
    log_info "Pool count increased as expected: $pool_count ✅"
  else
    log_warn "Pool count did not increase. Current: $pool_count ❎"
    EXIT_ERROR=1
  fi

  log_info "✅ CR update tests completed"
}

test_metrics() {
  log_info "=== Testing Metrics Endpoint ==="

  # Create service account for metrics access
  log_info "Creating metrics-reader service account..."
  kubectl create sa metrics-reader -n "$OPERATOR_NAMESPACE" 2>/dev/null || true

  # Bind to existing lbconfig-operator-metrics-reader ClusterRole (created by OLM bundle)
  kubectl create clusterrolebinding metrics-reader-binding \
    --clusterrole=lbconfig-operator-metrics-reader \
    --serviceaccount="$OPERATOR_NAMESPACE:metrics-reader" 2>/dev/null || true

  # Generate token for the service account (minimum 10 minutes required)
  local token
  token=$(kubectl create token metrics-reader -n "$OPERATOR_NAMESPACE" --duration=10m)

  if [[ -z "$token" ]]; then
    log_error "Could not create token for metrics-reader"
    return 1
  fi

  # Port forward to metrics service (runs in background)
  kubectl port-forward -n "$OPERATOR_NAMESPACE" svc/lbconfig-operator-controller-manager-metrics-service 8589:8443 &
  local pf_pid=$!

  sleep 3

  # Access metrics with bearer token - capture response for debugging
  local response
  response=$(curl -s -k -H "Authorization: Bearer $token" https://localhost:8589/metrics)

  if [[ "$VERBOSE" == "true" ]]; then
    log_info "Metrics response preview:"
    echo "$response" | head -20
  fi

  if echo "$response" | grep -q "externallb_total"; then
    log_info "Metrics endpoint accessible and contains operator metrics ✅"
  elif echo "$response" | grep -qi "unauthorized\|forbidden"; then
    log_warn "Metrics endpoint returned authentication error ❎"
    [[ "$VERBOSE" == "true" ]] && echo "$response"
  else
    log_warn "Metrics endpoint accessible but operator metrics not found ❎"
    EXIT_ERROR=1
    [[ "$VERBOSE" == "true" ]] && log_info "Response may contain other metrics or be empty"
  fi

  # Cleanup
  kill $pf_pid 2>/dev/null || true
  kubectl delete clusterrolebinding metrics-reader-binding 2>/dev/null || true
  kubectl delete sa metrics-reader -n "$OPERATOR_NAMESPACE" 2>/dev/null || true

  log_info "✅ Metrics test completed"
}

test_finalizer() {
  log_info "=== Testing Finalizer and Deletion ==="

  local cr_name="externalloadbalancer-master-dummy-test"

  # Check finalizer exists
  local finalizer=$(kubectl get elb "$cr_name" -n "$CR_NAMESPACE" -o jsonpath='{.metadata.finalizers[0]}')
  if [[ -n "$finalizer" ]]; then
    log_info "Finalizer present: $finalizer ✅"
  else
    log_warn "No finalizer found on CR ❎"
  fi

  # Delete CR
  log_info "Deleting ExternalLoadBalancer CR..."
  kubectl delete elb "$cr_name" -n "$CR_NAMESPACE" --timeout=30s

  # Verify deletion
  if kubectl get elb "$cr_name" -n "$CR_NAMESPACE" &>/dev/null; then
    log_error "CR still exists after deletion attempt ❎"
    EXIT_ERROR=1
  else
    log_info "CR successfully deleted ✅"
  fi

  log_info "✅ Finalizer test completed"
}

test_multiple_instances() {
  log_info "=== Testing Multiple ExternalLoadBalancer Instances ==="

  # Create second instance
  cat <<EOF | kubectl apply -f -
apiVersion: lb.lbconfig.carlosedp.com/v1
kind: ExternalLoadBalancer
metadata:
  name: externalloadbalancer-infra-dummy-test
  namespace: $CR_NAMESPACE
spec:
  vip: "10.0.0.50"
  nodelabels:
    "node-role.kubernetes.io/control-plane": ""
  ports:
    - 80
    - 443
  monitor:
    path: "/healthz"
    port: 1936
    monitortype: "http"
  provider:
    vendor: Dummy
    host: "https://10.0.0.1"
    port: 443
    creds: dummy-creds
    validatecerts: false
EOF

  sleep 5

  # Verify both instances can coexist
  check_resource "externalloadbalancer" "externalloadbalancer-master-dummy-test" "$CR_NAMESPACE"
  check_resource "externalloadbalancer" "externalloadbalancer-infra-dummy-test" "$CR_NAMESPACE"

  # Clean up second instance
  kubectl delete elb externalloadbalancer-infra-dummy-test -n "$CR_NAMESPACE"

  log_info "✅ Multiple instances test completed"
}

# Main test execution
main() {
  log_info "Starting E2E tests for lbconfig-operator"
  log_info "Operator Namespace: $OPERATOR_NAMESPACE"
  log_info "CR Namespace: $CR_NAMESPACE"
  log_info "Timeout: $TIMEOUT"

  # Run tests
  deploy_instance
  # test_operator_deployment
  # test_basic_cr_lifecycle
  # test_cr_update
  # test_metrics
  # test_multiple_instances
  test_node_addition
  test_finalizer

  if [[ $EXIT_ERROR -ne 0 ]]; then
    log_error "==================================="
    log_error "Some E2E tests failed ❎"
    log_error "==================================="
    exit 1
  fi

  log_info "==================================="
  log_info "✅ All E2E tests completed successfully!"
  log_info "==================================="
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
