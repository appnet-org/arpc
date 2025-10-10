# Set Go image
FROM golang:1.24.0-bullseye AS builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Create workspace
WORKDIR /app

COPY cmd/proxy/ ./
RUN go build -o proxy main.go buffer.go

# Final image
FROM ubuntu:22.04

WORKDIR /app
COPY --from=builder /app/proxy /app/proxy
RUN chmod +x /app/proxy