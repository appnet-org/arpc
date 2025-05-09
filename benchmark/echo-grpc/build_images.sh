#!/bin/bash
set -ex

go build -o server server.go
go build -o frontend frontend.go


# Build the frontend image
sudo docker build --tag echo-plain-frontend:latest -f Dockerfile-frontend ../..

# Build the server image
sudo docker build --tag echo-plain-server:latest -f Dockerfile-server ../..

# Tag the images
sudo docker tag echo-plain-frontend  appnetorg/echo-plain-frontend:latest
sudo docker tag echo-plain-server  appnetorg/echo-plain-server:latest

# Push the images to the registry
sudo docker push  appnetorg/echo-plain-frontend:latest
sudo docker push  appnetorg/echo-plain-server:latest

set +ex
