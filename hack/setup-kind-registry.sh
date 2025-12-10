#!/usr/bin/env bash
# Setup KIND cluster with local registry
# Based on: https://kind.sigs.k8s.io/docs/user/local-registry/

set -o errexit

# Desired cluster name
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-test-operator}"
KIND_BIN="${KIND_BIN:-./bin/kind}"
REG_NAME='kind-registry'
REG_PORT='5001'

# Ensure KIND network exists first
if ! podman network exists kind 2>/dev/null; then
  echo "Creating KIND network..."
  podman network create kind
fi

# Create registry container unless it already exists
if [ "$(podman inspect -f '{{.State.Running}}' "${REG_NAME}" 2>/dev/null || true)" != 'true' ]; then
  echo "Creating local registry container on KIND network..."
  podman run \
    -d --restart=always \
    -p "127.0.0.1:${REG_PORT}:5000" \
    --network=kind \
    --name "${REG_NAME}" \
    registry:2
  echo "✅ Local registry created at localhost:${REG_PORT}"
  sleep 2
else
  echo "✅ Local registry already running at localhost:${REG_PORT}"
fi

# Get the registry container IP from KIND network
REG_IP=$(podman inspect kind-registry --format '{{.NetworkSettings.Networks.kind.IPAddress}}' 2>/dev/null || echo "")
if [ -z "$REG_IP" ]; then
  echo "❌ Failed to get registry IP on KIND network"
  exit 1
fi

# Save registry info for Makefile to use (will be updated if KIND network is available)
echo "export E2E_REGISTRY_IP=${REG_IP}" >/tmp/kind-registry-info.env
echo "export E2E_REGISTRY_PORT=5000" >>/tmp/kind-registry-info.env

echo "Registry accessible at: ${REG_IP}:5000"

# Check if cluster already exists
if ${KIND_BIN} get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
  echo "✅ KIND cluster '${KIND_CLUSTER_NAME}' already exists"
  echo "Registry accessible at: ${REG_IP}:5000 in KIND network"

  # Cluster exists, we're done
  echo ""
  echo "================================================"
  echo "✅ Test environment ready!"
  echo "   Registry: localhost:${REG_PORT}"
  echo "   Cluster:  ${KIND_CLUSTER_NAME}"
  echo "================================================"
  exit 0
fi

# Create KIND cluster
echo "Creating KIND cluster '${KIND_CLUSTER_NAME}' with registry..."

cat <<EOF | ${KIND_BIN} create cluster --name "${KIND_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${REG_PORT}"]
    endpoint = ["http://${REG_IP}:5000"]
  [plugins."io.containerd.grpc.v1.cri".registry.configs."${REG_IP}:5000".tls]
    insecure_skip_verify = true
EOF

echo "✅ KIND cluster created"

# Registry is already on KIND network, just update the saved info
echo "Registry accessible at: ${REG_IP}:5000 in KIND network"
echo "✅ Registry configured for KIND cluster"

# Wait for cluster to be ready
echo "Waiting for cluster to be ready..."
kubectl wait --for=condition=ready node --all --timeout=60s

# Document the local registry
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REG_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

echo ""
echo "================================================"
echo "✅ Test environment ready!"
echo "   Registry: localhost:${REG_PORT}"
echo "   Cluster:  ${KIND_CLUSTER_NAME}"
echo "================================================"
