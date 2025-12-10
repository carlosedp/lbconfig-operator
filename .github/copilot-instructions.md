# LBConfig Operator - AI Coding Agent Instructions

## Project Overview

This is a Kubernetes operator that automates external load-balancer configuration (F5 BigIP, Citrix ADC/Netscaler, HAProxy) for Kubernetes/OpenShift clusters. It watches cluster nodes and dynamically configures VIPs, server pools, and health monitors on external LB hardware via their APIs.

**Key Concept**: The operator creates and maintains load balancer configurations that forward traffic to cluster nodes based on their roles (master/infra) or arbitrary labels, enabling external access to cluster services and APIs.

## Architecture

### Component Structure

```
internal/controller/
├── lb.lbconfig.carlosedp.com/          # Main reconciler
│   └── externalloadbalancer_controller.go
├── backend/
│   ├── backend_controller/              # Provider interface & orchestration
│   ├── backend_loader/                  # Auto-registers all providers via init()
│   ├── f5/                             # F5 BigIP implementation
│   ├── netscaler/                      # Citrix ADC implementation
│   ├── haproxy/                        # HAProxy Dataplane API implementation
│   └── dummy/                          # Testing provider
api/lb.lbconfig.carlosedp.com/v1/       # CRD types
```

### Provider Plugin Architecture

Backends are **plugin-based** via interface and auto-registration:

1. Each provider implements the `Provider` interface in `backend_controller/backend_controller.go` (CRUD for Monitors, Pools, PoolMembers, VIPs)
2. Providers call `RegisterProvider(name, providerInstance)` in their `init()` function
3. `backend_loader/backend_loader.go` imports all providers with `_` imports, triggering registration
4. Vendor names MUST match the enum in `api/v1/externalloadbalancer_types.go` Provider.Vendor field

**To add a new backend**:

- Create package in `internal/controller/backend/<name>/`
- Implement `Provider` interface (see dummy or f5 as examples)
- Add `RegisterProvider("Vendor_Name", new(YourProvider))` in `init()`
- Add blank import to `backend_loader/backend_loader.go`
- Add vendor name to `Provider.Vendor` enum in API types

### Reconciliation Flow

1. **Watch**: ExternalLoadBalancer CRs and Node events (via `SetupWithManager`)
2. **Node Selection**: Filter nodes by `.spec.type` (master/infra) OR `.spec.nodelabels` (custom label matching - all labels must match)
3. **Backend Orchestration**: `BackendController.HandleMonitors/HandlePool/HandleVIP` calls provider CRUD methods
4. **Finalizer Cleanup**: On CR deletion, remove LB configurations via `HandleCleanup`

## Development Workflows

### Local Development Cycle

```bash
# 1. Update CRD manifests after API changes
make bundle

# 2. Install CRDs to cluster
make install

# 3. Run operator locally (connects to cluster via kubeconfig)
make run
# Sets OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 for tracing
# Ignore Jaeger timeout errors unless you need tracing

# 4. Create test namespace and resources
kubectl create namespace lbconfig-operator-system
kubectl create secret generic dummy-creds \
  --from-literal=username=admin \
  --from-literal=password=admin \
  -n lbconfig-operator-system
kubectl apply -f config/samples/lb_v1_externalloadbalancer-dummy.yaml
```

### Testing

- **Unit tests**: `make test` - Uses Ginkgo + envtest (spawns k8s control plane)
- **KIND cluster**: `make olm-run` - Deploys operator via OLM in KIND cluster
- **Scorecard**: `make scorecard-run` - Validates operator with kuttl tests

### Build & Release

```bash
# Build all architectures and push images (amd64, arm64, ppc64le, s390x)
make dist

# For release:
git tag -a v$(make print-VERSION) -m "Release v$(make print-VERSION)"
git push origin v$(make print-VERSION)
```

## Critical Patterns & Conventions

### Node Label Selection Logic

```go
// type: "master" → node-role.kubernetes.io/master
// type: "infra" → node-role.kubernetes.io/infra
// nodelabels: map[string]string → ALL labels must match (AND logic)
```

Use `nodelabels` for router sharding or custom node selection instead of `type`.

### Naming Conventions in Load Balancers

Operator creates resources with predictable names (NEVER delete existing user configs):

- Pool: `Pool-<cr-name>-<port>`
- Monitor: `Monitor-<cr-name>`
- VIP: `VIP-<cr-name>-<port>`

### Backend Provider Requirements

- Implement all `Provider` interface methods (17 methods: Create, Connect, Close, Get/Create/Edit/Delete for Monitor/Pool/PoolMember/VIP)
- Map LB methods via `LBMethodMap` variable (see `f5_controller.go`)
- **Never delete pool members** - they may be shared across pools
- Backend logs disabled by default (set `BACKEND_LOGS` env var to enable)

### Tracing

- Uses OpenTelemetry SDK with Jaeger exporter
- Spans created in reconcile loop and backend operations
- Set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable (defaults to localhost:4317)

### Metrics

Prometheus metrics:

- `externallb_total`: Total ExternalLoadBalancer instances
- `externallb_nodes{name,namespace,type,vip,port,backend_vendor}`: Node count per LB instance

## Common Tasks

### Modifying CRD Spec

1. Edit `api/lb.lbconfig.carlosedp.com/v1/externalloadbalancer_types.go`
2. Add kubebuilder markers for validation/documentation
3. Run `make generate` then `make manifests` then `make bundle`
4. Update samples in `config/samples/`

### Debugging Backend Issues

- Check provider logs (enable `BACKEND_LOGS` env var)
- Use Dummy provider for logic testing without real LB
- Verify secret credentials exist in namespace
- Check `status.numnodes` and `status.labels` fields in CR

### Testing KIND Clusters

```bash
# Install KIND binary
make kind

# Create cluster
kind create cluster

# Operator uses current kubeconfig context
```

## Important Constraints

- Operator namespace MUST match where secrets are created
- Secrets MUST have `username` and `password` keys
- Provider vendor names are case-sensitive enums
- Multi-platform builds require Docker buildx or Podman manifest support
- Backend providers must handle their own connection management (Connect/Close)
