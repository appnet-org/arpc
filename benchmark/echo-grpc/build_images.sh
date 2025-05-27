#!/bin/bash
set -ex

# Build the frontend image
sudo docker build --tag echo-grpc-benchmark:latest .    

# Tag the images
sudo docker tag echo-grpc-benchmark  appnetorg/echo-grpc-benchmark:latest

# Push the images to the registry
sudo docker push  appnetorg/echo-grpc-benchmark:latest

set +ex
