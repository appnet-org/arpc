# Set Go image
FROM golang:1.24.0-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

# Copy the entire repo, assuming your context is ~/arpc
COPY . .

# Go to kv-store-symphony subdir
WORKDIR /app/benchmark/kv-store-symphony

RUN go mod tidy

# Build binaries
RUN go build -trimpath -ldflags="-s -w" -o ./bin/frontend ./frontend/frontend.go
RUN go build -trimpath -ldflags="-s -w" -o ./bin/kvstore ./kvstore/kvstore.go

# Final image
FROM ubuntu:22.04

WORKDIR /app

# Install iptables (and clean up to reduce image size)
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iptables && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy built binaries
COPY --from=builder /app/benchmark/kv-store-symphony/bin/frontend /app/frontend
COPY --from=builder /app/benchmark/kv-store-symphony/bin/kvstore /app/kvstore

# Make binaries executable
RUN chmod +x /app/frontend /app/kvstore

CMD ["/bin/bash"]
