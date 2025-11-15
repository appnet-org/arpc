# Set Go image
FROM golang:1.24.0-bullseye AS builder

# Accept build argument for variant (default: udp)
ARG VARIANT=udp

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

# Copy the entire repo, assuming your context is ~/arpc
COPY . .

# Go to kv-store-symphony-transport subdir
WORKDIR /app/benchmark/kv-store-symphony-transport

RUN go mod tidy

# Build binaries based on variant
RUN mkdir -p ./bin && \
    if [ "$VARIANT" = "udp" ]; then \
        go build -trimpath -ldflags="-s -w" -o ./bin/frontend ./frontend/frontend_udp.go && \
        go build -trimpath -ldflags="-s -w" -o ./bin/kvstore ./kvstore/kvstore_udp.go; \
    elif [ "$VARIANT" = "reliable" ]; then \
        go build -trimpath -ldflags="-s -w" -o ./bin/frontend ./frontend/frontend_reliable.go && \
        go build -trimpath -ldflags="-s -w" -o ./bin/kvstore ./kvstore/kvstore_reliable.go; \
    elif [ "$VARIANT" = "cc" ]; then \
        go build -trimpath -ldflags="-s -w" -o ./bin/frontend ./frontend/frontend_cc.go && \
        go build -trimpath -ldflags="-s -w" -o ./bin/kvstore ./kvstore/kvstore_cc.go; \
    elif [ "$VARIANT" = "reliable-cc" ]; then \
        go build -trimpath -ldflags="-s -w" -o ./bin/frontend ./frontend/frontend_reliable_cc.go && \
        go build -trimpath -ldflags="-s -w" -o ./bin/kvstore ./kvstore/kvstore_reliable_cc.go; \
    else \
        echo "Invalid VARIANT: $VARIANT" && exit 1; \
    fi

# Final image
FROM ubuntu:22.04

WORKDIR /app

# Install iptables (and clean up to reduce image size)
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iptables && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy built binaries
COPY --from=builder /app/benchmark/kv-store-symphony-transport/bin/frontend /app/frontend
COPY --from=builder /app/benchmark/kv-store-symphony-transport/bin/kvstore /app/kvstore

# Make binaries executable
RUN chmod +x /app/frontend /app/kvstore

CMD ["/bin/bash"]
