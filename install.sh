#!/bin/bash
set -e

# Install the Exasol Terraform Provider from GitHub Releases.
# Usage: curl -sfL https://raw.githubusercontent.com/exasol-labs/terraform-provider-exasol/main/install.sh | bash

REPO="exasol-labs/terraform-provider-exasol"

# Get latest version from GitHub
VERSION=${VERSION:-$(curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')}

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

DEST="$HOME/.terraform.d/plugins/registry.terraform.io/exasol/exasol/${VERSION}/${OS}_${ARCH}"
ASSET="terraform-provider-exasol_${VERSION}_${OS}_${ARCH}.zip"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ASSET}"

echo "Installing Exasol Terraform Provider v${VERSION} (${OS}/${ARCH})..."

# Check if already installed
if [ -f "${DEST}/terraform-provider-exasol_v${VERSION}" ]; then
    echo "Already installed at ${DEST}"
    exit 0
fi

# Download, extract, clean up
curl -fLo "/tmp/${ASSET}" "${URL}" || { echo "Download failed. Check https://github.com/${REPO}/releases"; exit 1; }
mkdir -p "${DEST}"
unzip -o "/tmp/${ASSET}" -d "${DEST}/"
rm "/tmp/${ASSET}"

echo "Installed to ${DEST}"
