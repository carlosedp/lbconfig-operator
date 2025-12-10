# Current Operator version
VERSION ?= 0.5.1
# VERSION ?= $(shell git describe --tags | sed 's/^v//') # Use this to get the latest tag
# Previous Operator version
PREV_VERSION ?= $(shell git describe --abbrev=0 --tags $(shell git rev-list --tags --skip=1 --max-count=1) | sed 's/^v//')

# E2E development version (for local testing, not for publishing)
# Changes patch to 0 and adds -dev suffix (e.g., 0.5.1 -> 0.5.0-dev)
E2E_VERSION ?= $(shell echo $(VERSION) | sed 's/\.[0-9]*$$/.0-dev/')
# For local testing with KIND, images will be tagged with localhost:5001 and pushed to container IP
E2E_IMG_NAME ?= lbconfig-operator:v$(E2E_VERSION)
E2E_BUNDLE_IMG_NAME ?= lbconfig-operator-bundle:v$(E2E_VERSION)
# These will be used by OLM - localhost:5001 is accessible from KIND nodes
LOCAL_REGISTRY ?= localhost:5001
E2E_IMG ?= $(LOCAL_REGISTRY)/$(E2E_IMG_NAME)
E2E_BUNDLE_IMG ?= $(LOCAL_REGISTRY)/$(E2E_BUNDLE_IMG_NAME)

# Tools version
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0
KUSTOMIZE_VERSION ?= v5.8.0
CONTROLLER_TOOLS_VERSION ?= v0.19.0
OPERATOR_SDK_VERSION ?= v1.36.1
OLM_VERSION ?= 0.38.0
KIND_VERSION ?= v0.30.0

MIN_KUBERNETES_VERSION ?= 1.19.0
MIN_OPENSHIFT_VERSION ?= 4.6

SED ?= "sed"

# Operator repository
REPO ?= quay.io/carlosedp

# Publishing channel
CHANNELS = "beta"
DEFAULT_CHANNEL = "beta"

# Architectures to build binaries and Docker images
ARCHS ?= amd64 arm64 ppc64le s390x
# Interpolated platform and architecture list for docker buildx separated by comma
# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> than the export will fail)
PLATFORMS = $(shell echo $(ARCHS) | sed -e 's~[^ ]*~linux/&~g' | tr ' ' ',')

# Which container runtime to use (check if Docker is available otherwise use Podman)
BUILDER = podman

# Image URL to use all building/pushing image targets
IMG ?= ${REPO}/lbconfig-operator:v$(VERSION)
IMAGE_TAG_BASE ?= ${REPO}/lbconfig-operator
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)
BUNDLE_IMGS ?= $(BUNDLE_IMG)
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)
# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: print-%
print-%: ## Print any variable from the Makefile. Use as `make print-VARIABLE`
	@echo $($*)

.PHONY: check-versions
check-versions: ## Check versions of tools
	@./hack/check_versions.sh

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate deployment manifests for manual install
.PHONY: deployment-manifests
deployment-manifests: manifests kustomize ## Generate deployment manifests for the controller.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > manifests/deploy.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ginkgo ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) run -r --randomize-all --randomize-suites --fail-on-pending --keep-going --trace --race --cover --covermode=atomic --coverprofile=coverage.out .

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.54.2
golangci-lint:
	@[ -f $(GOLANGCI_LINT) ] || { \
	set -e ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell dirname $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION) ;\
	}

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -a -installsuffix cgo \
		-ldflags '-X "main.Version=$(VERSION)" -s -w -extldflags "-static"' \
		-o output/manager ./cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317" go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image for the operator locally (linux/amd64).
	$(BUILDER) build -t ${IMG} --build-arg VERSION=${VERSION} --build-arg TARGETARCH=amd64 --build-arg TARGETOS=linux -f Dockerfile.cross .

.PHONY: docker-push
docker-push: ## Build and push docker image for the operator.
	$(BUILDER) push ${IMG}

.PHONY: docker-cross
docker-cross: ## Build operator binaries locally and then build/push the Docker image
	@for ARCH in $(ARCHS) ; do \
		OS=linux ; \
		echo "Building binary for $$ARCH at output/manager-$$OS-$$ARCH" ; \
		GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=0 \
		go build -a -installsuffix cgo \
		-ldflags '-X "main.Version=$(VERSION)" -s -w -extldflags "-static"' \
		-o output/manager-$$OS-$$ARCH ./cmd/main.go ; \
	done
	docker buildx build -t ${IMG} --platform=$(PLATFORMS) --push -f Dockerfile .

# To properly provided solutions that supports more than one platform you should use this option.
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross
	- docker buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: podman-crossbuild
podman-crossbuild: test ## Build and push a container image for the manager for cross-platform support with Podman
		@for ARCH in $(ARCHS) ; do \
		OS=linux ; \
		echo "Building binary for $$ARCH at output/manager-$$OS-$$ARCH" ; \
		GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=0 \
		go build -a -installsuffix cgo \
		-ldflags '-X "main.Version=$(VERSION)" -s -w -extldflags "-static"' \
		-o output/manager-$$OS-$$ARCH ./cmd/main.go ; \
	done
	podman manifest create ${IMG}
	podman build --platform $(PLATFORMS) --manifest ${IMG} -f Dockerfile .
	podman manifest push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GINKGO ?= $(LOCALBIN)/ginkgo
KIND ?= $(LOCALBIN)/kind
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	@test -s $(LOCALBIN)/kustomize || { \
		curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- $(shell echo $(KUSTOMIZE_VERSION) | sed 's/v//') $(LOCALBIN); \
	}


# Extract ginkgo version from go.mod removing the v prefix
GINKGO_VERSION = $(shell grep github.com/onsi/ginkgo go.mod | awk '{print $$2}' | sed 's/v//')

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	@test -s $(LOCALBIN)/ginkgo && $(LOCALBIN)/ginkgo version | grep -q $(GINKGO_VERSION) || GOBIN=$(LOCALBIN) go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo@v$(GINKGO_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	@test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
endif

.PHONY: kind
kind: ## Download kind locally if necessary.
	@{ \
	set -e ;\
	mkdir -p $(LOCALBIN) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION) ;\
	}

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: operator-sdk manifests kustomize deployment-manifests
	$(SED) -i 's/minKubeVersion: .*/minKubeVersion: $(MIN_KUBERNETES_VERSION)/' config/manifests/bases/lbconfig-operator.clusterserviceversion.yaml
	$(SED) -i 's/com.redhat.openshift.versions=.*/com.redhat.openshift.versions=v$(MIN_OPENSHIFT_VERSION)/' bundle.Dockerfile
	$(SED) -i 's/com.redhat.openshift.versions: .*/com.redhat.openshift.versions: v$(MIN_OPENSHIFT_VERSION)/' bundle/metadata/annotations.yaml
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --manifests --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	sed -i "s|containerImage:.*|containerImage: $(IMG)|g" "bundle/manifests/lbconfig-operator.clusterserviceversion.yaml"
	cp -rf ./config/kuttl ./bundle/tests/scorecard/
	$(OPERATOR_SDK) bundle validate -b $(BUILDER) ./bundle

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.55.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

.PHONY: bundle-build
bundle-build: bundle ## Build the bundle image.
	$(BUILDER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: bundle-build ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm bundle-push ## Build a catalog image.
	$(OPM) index add --container-tool $(BUILDER) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: catalog-build ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

.PHONY: olm-validate
olm-validate: operator-sdk ## Validates the bundle image.
	$(OPERATOR_SDK) bundle validate -b $(BUILDER) $(BUNDLE_IMG)

.PHONY: testenv-setup
testenv-setup: kind ## Setup the test environment (KIND cluster with local registry)
	@echo "Setting up local registry and KIND cluster..."
	@./hack/setup-kind-registry.sh

.PHONY: testenv-load-images
testenv-load-images: ## Push locally built images to local registry
	@if [ -f /tmp/kind-registry-info.env ]; then \
		echo "Pushing operator image to local registry (localhost:5001)..." && \
		$(BUILDER) push --tls-verify=false localhost:5001/$(E2E_IMG_NAME) && \
		echo "Pushing bundle image to local registry (localhost:5001)..." && \
		$(BUILDER) push --tls-verify=false localhost:5001/$(E2E_BUNDLE_IMG_NAME) && \
		echo "✅ Images pushed to local registry"; \
	else \
		echo "❌ Registry info not found. Run 'make testenv-setup' first."; \
		exit 1; \
	fi

.PHONY: e2e-push
e2e-push: ## Push e2e images to external registry (quay.io) - only for maintainers publishing dev builds
	@echo "Pushing operator image to quay.io..."
	$(BUILDER) tag $(E2E_IMG) ${REPO}/lbconfig-operator:v$(E2E_VERSION)
	$(BUILDER) push ${REPO}/lbconfig-operator:v$(E2E_VERSION)
	@echo "Pushing bundle image to quay.io..."
	$(BUILDER) tag $(E2E_BUNDLE_IMG) $(IMAGE_TAG_BASE)-bundle:v$(E2E_VERSION)
	$(BUILDER) push $(IMAGE_TAG_BASE)-bundle:v$(E2E_VERSION)

.PHONY: e2e-build
e2e-build: ## Build operator and bundle images for local e2e testing
	@echo "Building operator image for e2e testing (version: $(E2E_VERSION))..."
	$(BUILDER) build -t $(E2E_IMG) --build-arg VERSION=$(E2E_VERSION) --build-arg TARGETARCH=amd64 --build-arg TARGETOS=linux -f Dockerfile.cross .
	@echo "Building bundle for e2e testing..."
	$(MAKE) bundle VERSION=$(E2E_VERSION) IMG=$(E2E_IMG) BUNDLE_IMG=$(E2E_BUNDLE_IMG)
	@echo "Building bundle image for e2e testing..."
	$(BUILDER) build -f bundle.Dockerfile -t $(E2E_BUNDLE_IMG) .

.PHONY: e2e-test
e2e-test: operator-sdk e2e-build testenv-setup testenv-load-images ## Run full e2e tests with OLM via local registry
	@echo "Switching to KIND cluster context..."
	kubectl config use-context kind-test-operator
	@echo "Installing OLM..."
	$(OPERATOR_SDK) olm install --version=$(OLM_VERSION) --timeout=5m || true
	@echo "Deploying operator bundle ($(E2E_BUNDLE_IMG))..."
	$(OPERATOR_SDK) run bundle $(E2E_BUNDLE_IMG) --use-http --skip-tls-verify --timeout=5m
	@echo "Creating test resources..."
	kubectl create secret generic dummy-creds --from-literal=username=admin --from-literal=password=admin -n default || true
	kubectl apply -f config/samples/lb_v1_externalloadbalancer-dummy.yaml
	@echo "Waiting for operator to reconcile..."
	sleep 10
	@echo "Running comprehensive e2e test suite..."
	OPERATOR_NAMESPACE=default ./hack/e2e-tests.sh
	@echo "===================="
	@echo "✅ E2E tests completed successfully!"
	@echo "Cluster 'test-operator' is still running."
	@echo "To teardown: make testenv-teardown"
	@echo "To keep testing: kubectl config use-context kind-test-operator"
	@echo "===================="

.PHONY: e2e-test-quick
e2e-test-quick: operator-sdk e2e-build testenv-setup testenv-load-images ## Run quick e2e smoke test with OLM
	@echo "Switching to KIND cluster context..."
	kubectl config use-context kind-test-operator
	@echo "Installing OLM..."
	$(OPERATOR_SDK) olm install --version=$(OLM_VERSION) --timeout=5m || true
	@echo "Deploying operator bundle ($(E2E_BUNDLE_IMG))..."
	$(OPERATOR_SDK) run bundle $(E2E_BUNDLE_IMG) --use-http --skip-tls-verify --timeout=5m
	@echo "Creating test resources..."
	kubectl create secret generic dummy-creds --from-literal=username=admin --from-literal=password=admin -n default || true
	kubectl apply -f config/samples/lb_v1_externalloadbalancer-dummy.yaml
	@echo "Waiting for operator to reconcile..."
	sleep 10
	@echo "Quick validation..."
	kubectl get elb externalloadbalancer-master-dummy-test -n default -o yaml
	kubectl wait --for=jsonpath='{.status.numnodes}'=1 elb/externalloadbalancer-master-dummy-test -n default --timeout=60s
	@echo "Cleaning up..."
	kubectl delete -f config/samples/lb_v1_externalloadbalancer-dummy.yaml || true
	sleep 5
	$(OPERATOR_SDK) cleanup lbconfig-operator || true
	kubectl delete secret dummy-creds -n default || true
	@echo "===================="
	@echo "✅ Quick e2e test completed!"
	@echo "===================="

.PHONY: scorecard-run
scorecard-run: operator-sdk e2e-build testenv-setup testenv-load-images ## Runs the scorecard validation with locally built images
	kubectl config use-context kind-test-operator
	$(OPERATOR_SDK) olm install --version=$(OLM_VERSION) --timeout=5m || true
	$(OPERATOR_SDK) run bundle $(E2E_BUNDLE_IMG) --use-http --skip-tls-verify --timeout=5m || true
	kubectl create secret generic dummy-creds --from-literal=username=admin --from-literal=password=admin -n default || true
	$(OPERATOR_SDK) scorecard ./bundle --wait-time 5m --service-account=lbconfig-operator-controller-manager
	@echo "Cleaning up after scorecard..."
	$(OPERATOR_SDK) cleanup lbconfig-operator || true

.PHONY: testenv-teardown
testenv-teardown: ## Teardown the test environment (KIND cluster and local registry)
	@echo "Tearing down test environment..."
	@$(KIND) delete cluster --name test-operator 2>/dev/null || echo "KIND cluster not found"
	@$(BUILDER) stop kind-registry 2>/dev/null || echo "Registry container not running"
	@$(BUILDER) rm -f kind-registry 2>/dev/null || echo "Registry container not found"
	@rm -f /tmp/kind-registry-info.env 2>/dev/null || true
	@echo "✅ Test environment cleaned up"

.PHONY: dist
dist: check-versions bundle bundle-push catalog-push podman-crossbuild  ## Build manifests and container images, pushing them to the registry
	@sed -i -e 's|v[0-9]*\.[0-9]*\.[0-9]*|v$(VERSION)|g' Readme.md

.PHONY: clean
clean: ## Clean up all generated files
	rm -rf bin
	rm -rf output
	rm -rf lbconfig-operator
