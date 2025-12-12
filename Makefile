# Current Operator version
VERSION ?= 0.6.0
# Previous Operator version
PREV_VERSION ?= $(shell git describe --abbrev=0 --tags $(shell git rev-list --tags --skip=1 --max-count=1) | sed 's/^v//')

# Operator repository
REPO ?= quay.io/carlosedp

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

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Publishing channel
CHANNELS = "beta"
DEFAULT_CHANNEL = "beta"

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

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

# Architectures to build binaries and Docker images
ARCHS ?= amd64 arm64 ppc64le s390x
PLATFORMS = $(shell echo $(ARCHS) | sed -e 's~[^ ]*~linux/&~g' | tr ' ' ',')

# Which container runtime to use
BUILDER = podman

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
KIND ?= $(LOCALBIN)/kind
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GINKGO ?= $(LOCALBIN)/ginkgo
OPM ?= $(LOCALBIN)/opm

# Tools version
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
KUSTOMIZE_VERSION ?= v5.8.0
CONTROLLER_TOOLS_VERSION  ?= v0.19.0
KIND_VERSION ?= v0.30.0
OPERATOR_SDK_VERSION ?= v1.42.0
GOLANGCI_LINT_VERSION ?= v2.7.2
GINKGO_VERSION ?= v2.27.3
OPM_VERSION ?= v1.61.0
OLM_VERSION ?= 0.38.0

# Minimal Kubernetes and OpenShift Versions
MIN_KUBERNETES_VERSION ?= 1.19.0
MIN_OPENSHIFT_VERSION ?= 4.6

# Dynamically generated versions
ENVTEST_VERSION := $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION := $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

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

.PHONY: setup
setup: install-hooks ## Initial setup for development (install Git hooks)
	@echo "✅ Development environment setup complete"

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

.PHONY: install-hooks
install-hooks: ## Install Git hooks from the hooks/ directory
	@./hooks/install.sh

##@ Development

.PHONY: manifests
manifests: install-hooks controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Tests

# .PHONY: test
# test: manifests generate fmt vet setup-envtest ## Run tests.
# 	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test
test: install-hooks manifests generate fmt vet setup-envtest envtest ginkgo ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) run -r --randomize-all --randomize-suites --fail-on-pending --keep-going --trace --race --cover --covermode=atomic --coverprofile=coverage.out .

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: olm-validate
olm-validate: operator-sdk ## Validates the bundle image.
	$(OPERATOR_SDK) bundle validate -b $(BUILDER) $(BUNDLE_IMG)

# Below are targets to use Go e2e tests which are currently done further with other targets and a script in ./hack.
# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= lbconfig-operator-test-e2e

.PHONY: test-e2e
test-e2e: testenv-setup manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
# 	$(MAKE) cleanup-test-e2e

# .PHONY: cleanup-test-e2e
# cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
# 	@$(KIND) delete cluster --name $(KIND_CLUSTER)

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

.PHONY: e2e-build
e2e-build: ## Build operator and bundle images for local e2e testing
	@echo "Building operator image for e2e testing (version: $(E2E_VERSION))..."
	$(BUILDER) build -t $(E2E_IMG) --build-arg VERSION=$(E2E_VERSION) --build-arg TARGETARCH=amd64 --build-arg TARGETOS=linux -f Dockerfile.cross .
	@echo "Checking if bundle needs backup..."
	@if git diff --quiet bundle/ config/manager/kustomization.yaml config/manifests/ manifests/ 2>/dev/null; then \
		echo "Bundle unchanged, creating backup for restoration..." && \
		tar czf /tmp/bundle-backup.tar.gz bundle/ config/manager/kustomization.yaml config/manifests/ manifests/ 2>/dev/null; \
	else \
		echo "Bundle has local changes, will NOT restore after e2e-build" && \
		rm -f /tmp/bundle-backup.tar.gz; \
	fi
	@echo "Building bundle for e2e testing..."
	$(MAKE) bundle VERSION=$(E2E_VERSION) IMG=$(E2E_IMG) BUNDLE_IMG=$(E2E_BUNDLE_IMG)
	@echo "Building bundle image for e2e testing..."
	$(BUILDER) build -f bundle.Dockerfile -t $(E2E_BUNDLE_IMG) .
	@if [ -f /tmp/bundle-backup.tar.gz ]; then \
		echo "Restoring original bundle state..." && \
		tar xzf /tmp/bundle-backup.tar.gz && \
		rm -f /tmp/bundle-backup.tar.gz && \
		echo "✅ E2E images built (bundle/ restored to original state)"; \
	else \
		echo "✅ E2E images built (bundle/ contains updated manifests)"; \
	fi

.PHONY: e2e-test
e2e-test: operator-sdk e2e-build testenv-setup testenv-load-images ## Run full e2e tests with OLM via local registry
	@echo "Switching to KIND cluster context..."
	kubectl config use-context kind-test-operator
	@echo "Installing OLM..."
	$(OPERATOR_SDK) olm install --version=$(OLM_VERSION) --timeout=5m || true
	@echo "Creating operator namespace..."
	kubectl apply -f examples/namespace.yaml
	@echo "Cleaning up previous operator deployment (if any)..."
	@$(OPERATOR_SDK) cleanup lbconfig-operator --namespace lbconfig-operator-system --timeout=2m 2>/dev/null || true
	@kubectl delete catalogsource lbconfig-operator-catalog -n lbconfig-operator-system 2>/dev/null || true
	@sleep 5
	@echo "Deploying operator bundle ($(E2E_BUNDLE_IMG))..."
	$(OPERATOR_SDK) run bundle $(E2E_BUNDLE_IMG) --namespace lbconfig-operator-system --use-http --skip-tls-verify --timeout=5m
	@echo "Running comprehensive e2e test suite..."
	./hack/e2e-tests.sh
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
	@echo "Creating operator namespace..."
	kubectl apply -f examples/namespace.yaml
	@echo "Cleaning up previous operator deployment (if any)..."
	@$(OPERATOR_SDK) cleanup lbconfig-operator --namespace lbconfig-operator-system --timeout=2m 2>/dev/null || true
	@kubectl delete catalogsource lbconfig-operator-catalog -n lbconfig-operator-system 2>/dev/null || true
	@sleep 5
	@echo "Deploying operator bundle ($(E2E_BUNDLE_IMG))..."
	$(OPERATOR_SDK) run bundle $(E2E_BUNDLE_IMG) --namespace lbconfig-operator-system --use-http --skip-tls-verify --timeout=5m
	@echo "Creating test resources..."
	kubectl apply -f examples/secret_v1_creds.yaml
	kubectl apply -f examples/lb_v2_externalloadbalancer-dummy.yaml
	@echo "Waiting for operator to reconcile..."
	sleep 10
	@echo "Quick validation..."
	kubectl get elb externalloadbalancer-master-dummy-test -n default -o yaml
	kubectl wait --for=jsonpath='{.status.numnodes}'=1 elb/externalloadbalancer-master-dummy-test -n default --timeout=60s
	@echo "Cleaning up..."
	kubectl delete namespace lbconfig-operator-system || true
	sleep 5
	$(OPERATOR_SDK) cleanup lbconfig-operator --namespace lbconfig-operator-system || true
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
	@rm -f /tmp/bundle-backup.tar.gz 2>/dev/null || true
	@echo "✅ Test environment cleaned up"

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

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

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

##@ Tool Dependencies Install

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

.PHONY: kind
kind: $(KIND)
$(KIND): $(LOCALBIN)
	$(call go-install-tool,$(KIND),sigs.k8s.io/kind,$(KIND_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

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

##@ Bundle and Catalog generation for distribution

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: operator-sdk manifests kustomize build-installer
	sed -i 's/minKubeVersion: .*/minKubeVersion: $(MIN_KUBERNETES_VERSION)/' config/manifests/bases/lbconfig-operator.clusterserviceversion.yaml
	sed -i 's/com.redhat.openshift.versions=.*/com.redhat.openshift.versions=v$(MIN_OPENSHIFT_VERSION)/' bundle.Dockerfile
	sed -i 's/com.redhat.openshift.versions: .*/com.redhat.openshift.versions: v$(MIN_OPENSHIFT_VERSION)/' bundle/metadata/annotations.yaml
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS) -q --overwrite --manifests --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	sed -i "s|containerImage:.*|containerImage: $(IMG)|g" "bundle/manifests/lbconfig-operator.clusterserviceversion.yaml"
	cp -rf ./config/kuttl ./bundle/tests/scorecard/
	$(OPERATOR_SDK) bundle validate -b $(BUILDER) ./bundle

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

.PHONY: dist
dist: check-versions bundle bundle-push catalog-push podman-crossbuild  ## Build manifests and container images, pushing them to the registry
	@sed -i -e 's|v[0-9]*\.[0-9]*\.[0-9]*|v$(VERSION)|g' Readme.md

##@ Utils

.PHONY: clean
clean: ## Clean up all generated files
	rm -rf bin
	rm -rf output
	rm -rf lbconfig-operator
