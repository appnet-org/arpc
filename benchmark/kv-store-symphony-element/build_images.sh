#!/bin/bash
set -ex

# Get the absolute path to this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Push to repo root (assumes script is in examples/echo_symphony/)
pushd "${SCRIPT_DIR}/../../" > /dev/null

# Build settings
TAG="latest" 
DOCKERFILE_PATH="${SCRIPT_DIR}/Dockerfile"
IMAGE_NAME="kvstore-symphony"
FULL_IMAGE="appnetorg/${IMAGE_NAME}:${TAG}"

echo "Building image with tag: ${TAG}"

# Build the Docker image from the repo root
sudo docker build --network=host -f "${DOCKERFILE_PATH}" -t "${IMAGE_NAME}:${TAG}" .

# Tag and push
sudo docker tag "${IMAGE_NAME}:${TAG}" "${FULL_IMAGE}"
sudo docker push "${FULL_IMAGE}"

# Return to original directory
popd > /dev/null

set +ex
