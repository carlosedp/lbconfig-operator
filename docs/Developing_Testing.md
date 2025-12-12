# Developing and Testing

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

The easier way to test the operator locally is with a [KIND](https://kind.sigs.k8s.io/) cluster. Kind means Kubernetes In Docker and runs a complete 1-node cluster in your machine's Docker. Once installed, create a cluster with `kind create cluster`. It will configure the .kubeconfig appropriately.

There are multiple `make` targets available to ease development.

**To do "normal" develop-test cycle running the operator locally:**

1. Make sure the CRD manifests are updated with `make bundle`;
2. Install CRDs in the cluster: `make install`;
3. Run the operator locally: `make run`;
4. By default the operator will try to send traces to a local Jaeger. Either check the [tracing doc](Tracing.md) or ignore the timeout messages in the logs;
5. Create the namespace, secret and ExternalLoadBalancer CRs in cluster
   1. Create the operator namespace with `kubectl create namespace lbconfig-operator-system`;
   2. Create the secret (for example for dummy) with: `kubectl create secret generic -n lbconfig-operator-system dummy-creds --from-literal=username=admin --from-literal=password=admin`
   3. Create the CRs (for example for dummy backend) with: `kubectl apply -n lbconfig-operator-system -f examples/lb_v1_externalloadbalancer-dummy.yaml`

**To deploy the operator in the cluster as a pod, the steps are:**

1. Make sure the CRD manifests are updated with `make bundle`;
2. Install CRDs in the cluster: `make install`;
3. Deploy the operator manifests to the cluster (CRDs + Namespace + Operator Container): `make deploy`;
4. Create the namespace, secret and ExternalLoadBalancer CRs in cluster the same way as above.

To remove the manifests to the cluster use `make undeploy`. To uninstall the CRDs use `make uninstall`.

## Testing

There are three main test methods:

1 - Automated unit-tests

These tests run with `make test` and use [Ginkgo](https://onsi.github.io/ginkgo/) testing framework. The tests spawn a Kubernetes control-plane thru [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) and runs the operator logic on it.

2 - KIND cluster with Operator Lifecycle Manager

The makefile target `e2e-test` starts a KIND cluster, deploys OLM into it and then installs the operator. Check the [`Makefile`](../Makefile) for the commands used.

3 - Scorecard Tests

These tests also run against the previously deployed KIND cluster and does some default validations. It also uses kuttl tests to check the opetator deployed CustomResource. Check the `scorecard-run` target in the [`Makefile`](../Makefile).

## Distribute

Building the manifests and docker images: `make dist`.

Operator deployment manifest bundle is created at `./dist/install.yaml`.

The sample manifests for LoadBalancer instances and backends are in `./config/samples` folder.
