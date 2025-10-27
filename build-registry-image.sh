#!/usr/bin/env bash

set -euo pipefail

# Builds the registry service Docker image and pushes it to a container registry.
# Defaults to pushing to the local registry at localhost:5000.
#
# Usage:
#   ./build-registry-image.sh [REGISTRY_HOST[:PORT]] [IMAGE_NAME] [TAG]
# Example:
#   ./build-registry-image.sh localhost:5000 mcp-registry latest

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTEXT_DIR="${SCRIPT_DIR}/registry"

REGISTRY="${1:-localhost:5000}"
IMAGE_NAME="${2:-mcp-registry}"
TAG="${3:-latest}"

LOCAL_TAG="${IMAGE_NAME}:${TAG}"
REGISTRY_TAG="${REGISTRY}/${IMAGE_NAME}:${TAG}"

if [[ ! -f "${CONTEXT_DIR}/Dockerfile" ]]; then
  echo "Expected Dockerfile at ${CONTEXT_DIR}/Dockerfile" >&2
  exit 1
fi

echo "Building Docker image ${LOCAL_TAG} from ${CONTEXT_DIR}..."
docker build --pull -t "${LOCAL_TAG}" "${CONTEXT_DIR}"

echo "Tagging image as ${REGISTRY_TAG}..."
docker tag "${LOCAL_TAG}" "${REGISTRY_TAG}"

echo "Pushing ${REGISTRY_TAG}..."
docker push "${REGISTRY_TAG}"

echo "Done. Image available as ${REGISTRY_TAG}"
