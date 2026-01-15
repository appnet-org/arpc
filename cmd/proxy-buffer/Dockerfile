# Set Go image
FROM golang:1.24.0-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

# Copy the entire repository to handle the replace directive
COPY go.mod go.sum ./
COPY pkg/ ./pkg/
COPY cmd/proxy-buffer/ ./cmd/proxy-buffer/

# Build from the cmd/proxy directory
WORKDIR /app/cmd/proxy-buffer
RUN go build -trimpath -o myproxy

# Final image
FROM ubuntu:22.04

WORKDIR /app
COPY --from=builder /app/cmd/proxy-buffer/myproxy /app/proxy
RUN chmod +x /app/proxy