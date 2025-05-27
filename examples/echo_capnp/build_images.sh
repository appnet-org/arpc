#!/bin/bash
set -ex

# Build the frontend image
sudo docker build --tag echo-capnp:latest .

# Tag the images
sudo docker tag echo-capnp  appnetorg/echo-capnp:latest

# Push the images to the registry
sudo docker push  appnetorg/echo-capnp:latest

set +ex