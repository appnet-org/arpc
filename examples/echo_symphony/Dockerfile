# Set Go image
FROM golang:1.23.9-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

# Copy the entire repo, assuming your context is ~/arpc
COPY . .

# Go to echo_symphony subdir
WORKDIR /app/examples/echo_symphony

RUN go mod tidy

# Build binaries
RUN go build -trimpath -ldflags="-s -w" -o frontend ./frontend
RUN go build -trimpath -ldflags="-s -w" -o server ./server

# Final image
FROM ubuntu:22.04

WORKDIR /app

# Install iptables (and clean up to reduce image size)
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iptables && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy built binaries
COPY --from=builder /app/examples/echo_symphony/frontend /app/frontend
COPY --from=builder /app/examples/echo_symphony/server /app/server

# Make binaries executable
RUN chmod +x /app/frontend /app/server

CMD ["/bin/bash"]
