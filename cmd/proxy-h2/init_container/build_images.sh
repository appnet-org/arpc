#!/bin/bash
set -ex

# Get the absolute path to this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Push to repo root (assumes script is in examples/echo_symphony/)
pushd "${SCRIPT_DIR}/../../" > /dev/null

# Build settings
DOCKERFILE_PATH="${SCRIPT_DIR}/Dockerfile"
IMAGE_NAME="symphony-proxy-h2-init-container"
FULL_IMAGE="appnetorg/${IMAGE_NAME}:latest"

# Build the Docker image from the repo root
sudo docker build -f "${DOCKERFILE_PATH}" -t "${IMAGE_NAME}:latest" .

# Tag and push
sudo docker tag "${IMAGE_NAME}:latest" "${FULL_IMAGE}"
sudo docker push "${FULL_IMAGE}"

# Return to original directory
popd > /dev/null

set +ex
