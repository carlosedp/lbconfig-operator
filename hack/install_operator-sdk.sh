#!/bin/bash

VERSION=$(grep -oP 'OPERATOR_SDK_VERSION\s*\?=\s*\K.*' Makefile | sed 's/^v//')
SDK_VERSION=${VERSION:-1.36.1}

cd /tmp || exit
sudo apt-get install -y curl gpg
case $(uname -m) in
x86_64)
  ARCH=amd64
  ;;
aarch64)
  ARCH=arm64
  ;;
*)
  ARCH=$(uname -m)
  ;;
esac
OS=$(uname | awk '{print tolower($0)}')
OPERATOR_SDK_DL_URL=https://github.com/operator-framework/operator-sdk/releases/download/v${SDK_VERSION}
curl -LO "${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH}"
gpg --keyserver keyserver.ubuntu.com --recv-keys 052996E2A20B5C7E
curl -LO "${OPERATOR_SDK_DL_URL}/checksums.txt"
curl -LO "${OPERATOR_SDK_DL_URL}/checksums.txt.asc"
gpg -u "Operator SDK (release) <cncf-operator-sdk@cncf.io>" --verify checksums.txt.asc
grep "operator-sdk_${OS}_${ARCH}" checksums.txt | sha256sum -c -
chmod +x "operator-sdk_${OS}_${ARCH}" && sudo mv "operator-sdk_${OS}_${ARCH}" /usr/local/bin/operator-sdk
rm -f checksums.txt checksums.txt.asc "operator-sdk_${OS}_${ARCH}"
