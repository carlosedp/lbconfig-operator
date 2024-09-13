#!/bin/bash

# This script checks the latest versions of the tools used in the project
# listed in the Makefile

# Check if the Makefile exists
if [ ! -f Makefile ]; then
  echo "Makefile not found"
  exit 1
fi

# Get the list of tools from the Makefile
# ENVTEST_K8S_VERSION
# KUSTOMIZE_VERSION
# CONTROLLER_TOOLS_VERSION
# OPERATOR_SDK_VERSION
# OLM_VERSION
# KIND_VERSION

# Get the latest version of the tools
echo "Checking the latest versions of the tools used in the project"
# Golang version
GOLANGVERSION=$(curl -s https://go.dev/VERSION?m=text | head -1)

# ENVTEST_K8S_VERSION
ENVTEST_K8S_VERSION=$(curl -s https://api.github.com/repos/kubernetes/kubernetes/releases/latest | jq -r .tag_name | sed 's/^v//')

# KUSTOMIZE_VERSION
KUSTOMIZE_VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/kustomize/releases/latest | jq -r .tag_name | sed 's/^kustomize\///')

# CONTROLLER_TOOLS_VERSION
CONTROLLER_TOOLS_VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/controller-tools/releases/latest | jq -r .tag_name)

# OPERATOR_SDK_VERSION
OPSDKVERSION=$(curl -s https://api.github.com/repos/operator-framework/operator-sdk/releases/latest | jq -r .tag_name)

# OLM_VERSION
OLMVER=$(curl -s https://api.github.com/repos/operator-framework/operator-lifecycle-manager/releases/latest | jq -r .tag_name | sed 's/^v//')

# KIND_VERSION
KINDVERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | jq -r .tag_name)

# Print current versions from the Makefile and the latest versions
CURRGO=$(grep "go" go.mod | head -1 | awk '{print $2}')
echo "Golang version: $CURRGO -> $GOLANGVERSION"
CURRK8S=$(grep "ENVTEST_K8S_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "K8S_VERSION: $CURRK8S -> $ENVTEST_K8S_VERSION"
CURRKUST=$(grep "KUSTOMIZE_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "KUSTOMIZE_VERSION: $CURRKUST -> $KUSTOMIZE_VERSION"
CURRCT=$(grep "CONTROLLER_TOOLS_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "CONTROLLER_TOOLS_VERSION: $CURRCT -> $CONTROLLER_TOOLS_VERSION"
CURROPSDK=$(grep "OPERATOR_SDK_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "OPERATOR_SDK_VERSION: $CURROPSDK -> $OPSDKVERSION"
CURROLM=$(grep "OLM_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "OLM_VERSION: $CURROLM -> $OLMVER"
CURRKIND=$(grep "KIND_VERSION" Makefile | awk -F= '{print $2}' | grep -v '^$' | head -1)
echo "KIND_VERSION: $CURRKIND -> $KINDVERSION"
