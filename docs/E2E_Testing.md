# End-to-End Testing Guide

This document describes the end-to-end (e2e) testing strategy for the lbconfig-operator.

## Overview

The e2e tests validate the operator's behavior in a real Kubernetes cluster (KIND) using locally built images. Unlike unit tests that use envtest, e2e tests deploy the full operator stack including OLM, bundle, and operator container.

## Quick Start

```bash
# Run full e2e test suite
make e2e-test

# Run quick smoke test only
make e2e-test-quick

# Cleanup
make testenv-teardown
```

## Testing Strategy

### 1. Unit Tests (`make test`)
- Framework: Ginkgo + Gomega
- Environment: envtest (in-memory k8s control plane)
- Coverage: Individual controller logic, backend providers, API validation
- Fast feedback for development

### 2. E2E Tests (`make e2e-test`)
- Framework: Custom bash test script (`hack/e2e-tests.sh`)
- Environment: KIND cluster with OLM
- Coverage: Full operator lifecycle, multi-instance scenarios, status updates
- Validates real-world deployment scenarios

### 3. Scorecard Tests (`make scorecard-run`)
- Framework: Operator SDK scorecard + kuttl
- Environment: KIND cluster with OLM
- Coverage: Operator best practices, OLM integration
- Validates operator metadata and bundle

## E2E Test Workflow

### Build Phase (`e2e-build`)

**Important**: E2E builds use a development version suffix (`-dev`) to distinguish from published releases. The patch version is set to 0 and `-dev` suffix is added. For example, if `VERSION=0.5.1`, e2e builds will be tagged as `v0.5.0-dev`. This ensures:
- No confusion with official releases
- No accidental usage of development images
- Clear separation between published and test artifacts

The build process:
1. Compiles operator binary for current architecture with dev version (e.g., `0.5.0-dev`)
2. Builds Docker image tagged with dev version (e.g., `v0.5.0-dev`)
3. Generates bundle manifests with dev version
4. Builds bundle image tagged with dev version

**Image naming example** (VERSION=0.5.1):
- Operator: `quay.io/carlosedp/lbconfig-operator:v0.5.0-dev`
- Bundle: `quay.io/carlosedp/lbconfig-operator-bundle:v0.5.0-dev`

These images are built locally and loaded directly into KIND - they are **never pushed to a registry**.

### Setup Phase
1. `testenv-setup`: Creates KIND cluster named `test-operator` (idempotent)
2. `testenv-load-images`: Loads local images into KIND (no registry needed)

### Deployment Phase
1. Switches kubectl context to KIND cluster
2. Installs OLM into cluster
3. Deploys operator via `operator-sdk run bundle`
4. Creates test secret and ExternalLoadBalancer CR

### Test Phase (`hack/e2e-tests.sh`)

The test script validates:

#### Operator Deployment
- Pod running and healthy
- No critical errors in logs

#### Basic CR Lifecycle
- CR created successfully
- Status fields populated (numnodes, monitor, pools)
- Nodes discovered based on labels

#### CR Updates
- VIP changes reflected
- Port additions create new pools
- Status updates accordingly

#### Metrics
- Prometheus metrics endpoint accessible
- Operator-specific metrics present

#### Multi-Instance
- Multiple ExternalLoadBalancer CRs can coexist
- Independent configuration management

#### Finalizers
- Finalizer added on creation
- Cleanup executed on deletion
- CR removed cleanly

## Test Targets

### `make e2e-test`
Full test suite with comprehensive validation.

**Use case**: Pre-merge validation, regression testing

**Duration**: ~3-5 minutes

**Cleanup**: Leaves cluster running for debugging

### `make e2e-test-quick`
Basic smoke test without deep validation.

**Use case**: Quick validation during development

**Duration**: ~1-2 minutes

**Cleanup**: Destroys test resources automatically

### `make scorecard-run`
Runs Operator SDK scorecard tests.

**Use case**: Pre-release validation, OLM certification

**Duration**: ~5-10 minutes

## Manual Testing in KIND

Keep the cluster running for manual validation:

```bash
# Run e2e tests (leaves cluster running)
make e2e-test

# Switch to KIND context
kubectl config use-context kind-test-operator

# Create additional test resources
kubectl apply -f config/samples/lb_v1_externalloadbalancer-infra.yaml

# Check operator logs
kubectl logs -n lbconfig-operator-system -l control-plane=controller-manager -f

# Inspect CR status
kubectl get elb -n lbconfig-operator-system -o yaml

# Cleanup when done
make testenv-teardown
```

## Debugging Failed Tests

### Check Operator Logs
```bash
kubectl logs -n lbconfig-operator-system -l control-plane=controller-manager --tail=100
```

### Inspect CR Status
```bash
kubectl get elb -n lbconfig-operator-system -o yaml
```

### Check Events
```bash
kubectl get events -n lbconfig-operator-system --sort-by='.lastTimestamp'
```

### Describe Deployment
```bash
kubectl describe deployment -n lbconfig-operator-system lbconfig-operator-controller-manager
```

### Check OLM Resources
```bash
kubectl get csv -n lbconfig-operator-system
kubectl get installplan -n lbconfig-operator-system
kubectl get subscription -n lbconfig-operator-system
```

## Extending Tests

### Adding New Test Cases

Edit `hack/e2e-tests.sh` and add a new test function:

```bash
test_my_new_feature() {
    log_info "=== Testing My New Feature ==="
    
    # Test implementation
    
    log_info "✅ My feature test completed"
}
```

Add the function call to `main()`:

```bash
main() {
    # ... existing tests ...
    test_my_new_feature
    # ...
}
```

### Adding Kuttl Tests

Kuttl tests are in `config/kuttl/` and run via scorecard.

Create a new test directory:
```bash
mkdir config/kuttl/test-my-feature
```

Add test steps:
- `00-install.yaml` - Prerequisites
- `01-assert.yaml` - Assertions
- `10-create.yaml` - Create resources
- `11-assert.yaml` - Validate state
- `99-cleanup.yaml` - Cleanup

## CI Integration

For GitHub Actions or other CI systems:

```yaml
- name: Run E2E Tests
  run: |
    make e2e-test
    make testenv-teardown
```

## Troubleshooting

### KIND cluster fails to create
- Check Docker/Podman is running
- Ensure port 6443 is available
- Try: `kind delete cluster --name test-operator` then retry

### Images fail to load
- Ensure images built successfully: `podman images | grep lbconfig-operator`
- Check image names match dev version: `echo $E2E_IMG` and `echo $E2E_BUNDLE_IMG`
- Should see tags like `v0.5.0-dev` (patch set to 0), not `v0.5.1`
- Manually load: `kind load docker-image <image> --name test-operator`

### OLM install fails
- Check cluster has enough resources
- Try manual OLM install: `operator-sdk olm install --version=0.38.0`
- Verify OLM pods: `kubectl get pods -n olm`

### Bundle deployment fails
- Validate bundle: `operator-sdk bundle validate ./bundle`
- Check CSV: `cat bundle/manifests/lbconfig-operator.clusterserviceversion.yaml`
- Verify CRDs installed: `kubectl get crd | grep lbconfig`

### Tests fail with timeout
- Increase timeout in test script: `export TIMEOUT=300s`
- Check operator pod status: `kubectl get pods -n lbconfig-operator-system`
- Review operator logs for errors

## Best Practices

1. **Run tests before commits**: `make test && make e2e-test-quick`
2. **Full validation before PR**: `make test && make e2e-test && make scorecard-run`
3. **Keep cluster for debugging**: Don't run `testenv-teardown` immediately after failures
4. **Test with multiple backends**: Not just Dummy - test F5, Netscaler, HAProxy when available
5. **Validate status fields**: Ensure status reflects actual LB state
6. **Test edge cases**: Missing secrets, invalid CRs, node label changes

## Future Improvements

- [ ] Add performance/load testing
- [ ] Test with real LB backends (F5, Netscaler, HAProxy) in CI
- [ ] Add chaos testing (node failures, network partitions)
- [ ] Automated upgrade testing (v0.4.x → v0.5.x)
- [ ] Multi-cluster testing scenarios
- [ ] Coverage reporting for e2e tests
- [ ] Parallel test execution
