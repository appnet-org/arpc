#!/bin/bash
set -e

# Get the absolute path to this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Push to repo root (assumes script is in benchmark/kv-store-symphony-transport/)
pushd "${SCRIPT_DIR}/../../" > /dev/null

# Build settings
DOCKERFILE_PATH="${SCRIPT_DIR}/Dockerfile"
IMAGE_NAME="kvstore-symphony-transport"

# Define all variants
VARIANTS=("udp" "reliable" "cc" "reliable-cc")

# Build and push each variant
for VARIANT in "${VARIANTS[@]}"; do
    echo "=========================================="
    echo "Building variant: ${VARIANT}"
    echo "=========================================="
    
    # Build the Docker image with variant-specific build arg
    sudo docker build --network=host \
        --build-arg VARIANT="${VARIANT}" \
        -f "${DOCKERFILE_PATH}" \
        -t "${IMAGE_NAME}:${VARIANT}" \
        .
    
    # Tag for DockerHub
    FULL_IMAGE="appnetorg/${IMAGE_NAME}:${VARIANT}"
    sudo docker tag "${IMAGE_NAME}:${VARIANT}" "${FULL_IMAGE}"
    
    # Push to DockerHub
    sudo docker push "${FULL_IMAGE}"
    
    echo "Successfully built and pushed: ${FULL_IMAGE}"
    echo ""
done

echo "=========================================="
echo "All variants built and pushed successfully!"
echo "=========================================="

# Return to original directory
popd > /dev/null

set +ex
