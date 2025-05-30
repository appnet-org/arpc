# aRPC vs gRPC Benchmark

This directory contains benchmarks comparing the performance of aRPC and gRPC using a simple echo server under various scenarios.

> **Note**: The benchmark scripts have been tested on Ubuntu 20.04.

## Running Benchmarks

### Prerequisites
- Go 1.20 or later
- wrk (for load testing)
- Docker and Kubernetes 
    - Run `. ./scripts/k8s_setup.sh` if on Ubuntu 20.04

For consistent results, it's recommended to run `. ./perf_setup.sh`

### Setup

Build Docker images and push it to DockerHub (optional):
```bash
# Build gRPC images (TODO: change docker username and run docker login)
cd echo-grpc
bash build_images.sh
cd -

# Build aRPC images
cd ../examples/echo_capnp
bash build_images.sh
cd -
```

### Running Tests

1. Benchmark gRPC
```bash
# Deploy gRPC application
kubectl apply -f echo-grpc/echo-grpc.yaml

# Basic test
curl "http://10.96.88.88:80?key=hello&header=1"

# Run wrk for latency test
./wrk/wrk -d 20s -t 1 -c 1 http://10.96.88.88:80 -s wrk.lua -L

# Clean up
kubectl delete -f echo-grpc/echo-grpc.yaml
```

2. Benchmark aRPC
```bash
# Deploy gRPC application
kubectl apply -f ../examples/echo_capnp/echo_capnp.yaml

# Basic test
curl "http://10.96.88.88:80?key=hello&header=1"

# Run wrk for latency test
./wrk/wrk -d 20s -t 1 -c 1 http://10.96.88.88:80 -s wrk.lua -L

# Clean up
kubectl delete -f ../examples/echo_capnp/echo_capnp.yaml
```


### Results



Framework|Latency(median)|Latency(mean)|Latency(P99)|Latency(max)|QPS
-------------|-------------|-------------|-------------|-------------|-------------
aRPC|0.98ms|1.10ms|2.07ms|2.84ms|962
gRPC|1.89ms|2.02ms|3.03ms|6.03ms|495

